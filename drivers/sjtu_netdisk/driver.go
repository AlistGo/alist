package sjtu_netdisk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type SJTUNetdisk struct {
	model.Storage
	Addition
	ref *SJTUNetdisk

	accessToken  string
	libraryId    string
	spaceId      string
	tokenExpires time.Time
	tokenMutex   sync.Mutex
}

func (d *SJTUNetdisk) Config() driver.Config {
	return config
}

func (d *SJTUNetdisk) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *SJTUNetdisk) Init(ctx context.Context) error {
	return d.refreshToken(ctx)
}

func (d *SJTUNetdisk) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*SJTUNetdisk)
	if ok {
		d.ref = refStorage
		return nil
	}
	return errs.NotSupport
}

func (d *SJTUNetdisk) Drop(ctx context.Context) error {
	d.ref = nil
	return nil
}

func (d *SJTUNetdisk) GetRoot(ctx context.Context) (model.Obj, error) {
	return &model.Object{
		ID:       "root",
		Path:     "/",
		Name:     "root",
		Size:     0,
		IsFolder: true,
	}, nil
}

func (d *SJTUNetdisk) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	listURL := fmt.Sprintf("%s/directory/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(dir.GetPath()))

	var allObjs []model.Obj
	page := 1

	for {
		var resp FolderListResp
		_, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParams(map[string]string{
				"access_token":  d.accessToken,
				"page":          strconv.Itoa(page),
				"page_size":     "200",
				"order_by":      d.OrderBy,
				"order_by_type": d.OrderByType,
				"space_org_id":  "1",
			}).
			SetResult(&resp).
			Execute(http.MethodGet, listURL)

		if err != nil {
			return nil, err
		}

		objs, err := utils.SliceConvert(resp.Contents, func(src NetdiskObj) (model.Obj, error) {
			fileSize, _ := strconv.ParseInt(src.Size, 10, 64)

			var objPath string
			dirPath := dir.GetPath()
			if dirPath == "" || dirPath == "/" {
				objPath = "/" + src.Name
			} else {
				objPath = path.Join(dirPath, src.Name)
			}

			return &model.Object{
				Name:     src.Name,
				Size:     fileSize,
				IsFolder: src.Type != "file",
				Modified: src.ModificationTime,
				Ctime:    src.CreationTime,
				Path:     objPath,
			}, nil
		})

		if err != nil {
			return nil, err
		}

		allObjs = append(allObjs, objs...)

		// stop if we have fetched all items
		if len(allObjs) >= resp.TotalNum || len(resp.Contents) < 200 {
			break
		}
		page++
	}

	return allObjs, nil
}

func (d *SJTUNetdisk) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	linkURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(file.GetPath()))

	client := d.newClient()
	client.SetRedirectPolicy(resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}))

	resp, err := client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"access_token":        d.accessToken,
			"user_id":             d.UserId,
			"content_disposition": "attachment",
			"purpose":             "download",
			"space_org_id":        "1",
		}).
		Execute(http.MethodGet, linkURL)

	if err != nil {
		return nil, err
	}

	// status code is not 301 and 302
	if resp.StatusCode() != http.StatusFound && resp.StatusCode() != http.StatusMovedPermanently {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	s3URL := resp.Header().Get("Location")
	if s3URL == "" {
		return nil, fmt.Errorf("no Location header in redirect response")
	}

	// parse TTL from S3 presigned URL's X-Amz-Expires parameter
	ttl := 2 * time.Hour
	if u, parseErr := url.Parse(s3URL); parseErr == nil {
		if expiresSec, convErr := strconv.Atoi(u.Query().Get("X-Amz-Expires")); convErr == nil && expiresSec > 0 {
			ttl = time.Duration(expiresSec) * time.Second
		}
	}

	return &model.Link{
		URL:        s3URL,
		Expiration: &ttl,
	}, nil
}

func (d *SJTUNetdisk) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	infoURL := fmt.Sprintf("%s/directory/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(parentDir.GetPath()+"/"+dirName))

	resp, err := d.newClient().R().
		SetContext(ctx).
		SetQueryParam("access_token", d.accessToken).
		SetQueryParam("user_id", d.UserId).
		SetQueryParam("conflict_resolution_strategy", "ask").
		Execute(http.MethodPut, infoURL)

	if err != nil {
		return nil, err
	}

	// 409 means folder with the same name exists (treated as success for idempotency); 201 means created
	if resp.StatusCode() != http.StatusConflict && resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return &model.Object{
		Name:     dirName,
		IsFolder: true,
		Modified: time.Now(),
		Ctime:    time.Now(),
		Path:     buildPath(parentDir.GetPath(), dirName),
	}, nil
}

func (d *SJTUNetdisk) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	srcPath := srcObj.GetPath()
	targetPath := path.Join(dstDir.GetPath(), srcObj.GetName())

	// file move: use overwrite strategy
	if !srcObj.IsDir() {
		apiURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(targetPath))

		var moveResp MoveCopyResp
		_, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParam("access_token", d.accessToken).
			SetQueryParam("conflict_resolution_strategy", "overwrite").
			SetBody(map[string]interface{}{"from": srcPath}).
			SetResult(&moveResp).
			Execute(http.MethodPut, apiURL)

		if err != nil {
			return nil, err
		}

		actualName := srcObj.GetName()
		if len(moveResp.Path) > 0 {
			actualName = moveResp.Path[len(moveResp.Path)-1]
		}
		return &model.Object{
			Name:     actualName,
			Size:     srcObj.GetSize(),
			IsFolder: false,
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
			Path:     buildPath(dstDir.GetPath(), actualName),
		}, nil
		// folder move
	} else {
		// use ask strategy to detect conflict
		err := d.moveItem(ctx, srcPath, dstDir.GetPath(), srcObj.GetName(), true, "ask")
		if err == nil {
			return &model.Object{
				Name:     srcObj.GetName(),
				Size:     srcObj.GetSize(),
				IsFolder: true,
				Modified: srcObj.ModTime(),
				Ctime:    srcObj.CreateTime(),
				Path:     targetPath,
			}, nil
		}

		// 409 means conflict name, needs merge
		if !errors.Is(err, errs.ObjectNotFound) {
			return nil, err
		}

		// list all child item of srcObj
		children, listErr := d.List(ctx, srcObj, model.ListArgs{})
		if listErr != nil {
			return nil, fmt.Errorf("merge: list source folder failed: %w", listErr)
		}

		// move every child item to dstDir recursively
		for _, child := range children {
			childSrcPath := path.Join(srcPath, child.GetName())
			targetDstPath := targetPath

			// subfolder: recursive Move
			if child.IsDir() {
				childErr := d.moveItem(ctx, childSrcPath, targetDstPath, child.GetName(), true, "ask")
				if childErr != nil && errors.Is(childErr, errs.ObjectNotFound) {
					childObj := &model.Object{
						Name:     child.GetName(),
						Path:     childSrcPath,
						Size:     child.GetSize(),
						IsFolder: true,
					}
					_, mergeErr := d.Move(ctx, childObj, &model.Object{
						Path:     targetPath,
						Name:     srcObj.GetName(),
						IsFolder: true,
					})
					if mergeErr != nil {
						return nil, fmt.Errorf("merge subfolder %s: %w", child.GetName(), mergeErr)
					}
				} else if childErr != nil {
					return nil, fmt.Errorf("merge subfolder %s: %w", child.GetName(), childErr)
				}
			} else {
				// child item is file
				fileErr := d.moveItem(ctx, childSrcPath, targetDstPath, child.GetName(), false, "overwrite")
				if fileErr != nil {
					return nil, fmt.Errorf("merge file %s: %w", child.GetName(), fileErr)
				}
			}
		}

		// delete empty srcObj folder
		_ = d.Remove(ctx, srcObj)

		return &model.Object{
			Name:     srcObj.GetName(),
			Size:     srcObj.GetSize(),
			IsFolder: true,
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
			Path:     targetPath,
		}, nil
	}
}

func (d *SJTUNetdisk) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	parentPath := path.Dir(strings.ReplaceAll(srcObj.GetPath(), "\\", "/"))
	dstPath := buildPath(parentPath, newName)

	// file rename
	if !srcObj.IsDir() {
		renameURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(dstPath))

		var moveResp MoveCopyResp
		_, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParam("access_token", d.accessToken).
			SetQueryParam("conflict_resolution_strategy", "overwrite").
			SetBody(map[string]interface{}{"from": srcObj.GetPath()}).
			SetResult(&moveResp).
			Execute(http.MethodPut, renameURL)

		if err != nil {
			return nil, err
		}

		actualName := newName
		if len(moveResp.Path) > 0 {
			actualName = moveResp.Path[len(moveResp.Path)-1]
		}
		return &model.Object{
			Name:     actualName,
			Size:     srcObj.GetSize(),
			IsFolder: false,
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
			Path:     buildPath(parentPath, actualName),
		}, nil
	} else {
		// folder rename
		renameURL := fmt.Sprintf("%s/directory/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(dstPath))

		resp, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParam("access_token", d.accessToken).
			SetQueryParam("conflict_resolution_strategy", "ask").
			SetQueryParam("move_authority", "true").
			SetBody(map[string]interface{}{"from": srcObj.GetPath()}).
			Execute(http.MethodPut, renameURL)

		if err != nil {
			return nil, err
		}

		// 204 means no conflict
		if resp.StatusCode() == http.StatusNoContent {
			return &model.Object{
				Name:     newName,
				Size:     srcObj.GetSize(),
				IsFolder: true,
				Modified: srcObj.ModTime(),
				Ctime:    srcObj.CreateTime(),
				Path:     dstPath,
			}, nil
		}

		// 409 means conflict name
		if resp.StatusCode() == http.StatusConflict {
			children, listErr := d.List(ctx, srcObj, model.ListArgs{})
			if listErr != nil {
				return nil, fmt.Errorf("rename merge: list source folder failed: %w", listErr)
			}

			targetDirObj := &model.Object{Path: dstPath, Name: newName, IsFolder: true}

			for _, child := range children {
				childSrcPath := path.Join(srcObj.GetPath(), child.GetName())

				if child.IsDir() {
					childObj := &model.Object{
						Name: child.GetName(), Path: childSrcPath,
						Size: child.GetSize(), IsFolder: true,
					}
					if _, mergeErr := d.Move(ctx, childObj, targetDirObj); mergeErr != nil {
						return nil, fmt.Errorf("rename merge subfolder %s: %w", child.GetName(), mergeErr)
					}
				} else {
					fileErr := d.moveItem(ctx, childSrcPath, dstPath, child.GetName(), false, "overwrite")
					if fileErr != nil {
						return nil, fmt.Errorf("rename merge file %s: %w", child.GetName(), fileErr)
					}
				}
			}

			_ = d.Remove(ctx, srcObj)

			return &model.Object{
				Name:     newName,
				Size:     srcObj.GetSize(),
				IsFolder: true,
				Modified: srcObj.ModTime(),
				Ctime:    srcObj.CreateTime(),
				Path:     dstPath,
			}, nil
		}

		return nil, fmt.Errorf("unexpected rename folder response: status=%d", resp.StatusCode())
	}
}

func (d *SJTUNetdisk) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	srcPath := srcObj.GetPath()
	targetPath := path.Join(dstDir.GetPath(), srcObj.GetName())

	// file copy
	if !srcObj.IsDir() {
		copyURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(targetPath))

		var copyResp MoveCopyResp
		_, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParam("access_token", d.accessToken).
			SetQueryParam("conflict_resolution_strategy", "overwrite").
			SetBody(map[string]interface{}{"copyFrom": srcPath}).
			SetResult(&copyResp).
			Execute(http.MethodPut, copyURL)

		if err != nil {
			return nil, err
		}

		actualName := srcObj.GetName()
		if len(copyResp.Path) > 0 {
			actualName = copyResp.Path[len(copyResp.Path)-1]
		}
		return &model.Object{
			Name:     actualName,
			Size:     srcObj.GetSize(),
			IsFolder: false,
			Modified: srcObj.ModTime(),
			Ctime:    time.Now(),
			Path:     buildPath(dstDir.GetPath(), actualName),
		}, nil
	} else {
		// folder copy
		infoURL := fmt.Sprintf("%s/directory/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(targetPath))

		resp, err := d.newClient().R().
			SetContext(ctx).
			SetQueryParam("access_token", d.accessToken).
			SetQueryParam("conflict_resolution_strategy", "ask").
			SetBody(map[string]interface{}{"copyFrom": srcPath}).
			Execute(http.MethodPut, infoURL)

		if err != nil {
			return nil, err
		}

		if resp.StatusCode() == http.StatusConflict {
			children, listErr := d.List(ctx, srcObj, model.ListArgs{})
			if listErr != nil {
				return nil, fmt.Errorf("copy merge: list source folder failed: %w", listErr)
			}

			targetDirObj := &model.Object{
				Path:     targetPath,
				Name:     srcObj.GetName(),
				IsFolder: true,
			}

			for _, child := range children {
				childSrcPath := path.Join(srcPath, child.GetName())

				if child.IsDir() {
					childObj := &model.Object{
						Name:     child.GetName(),
						Path:     childSrcPath,
						Size:     child.GetSize(),
						IsFolder: true,
					}
					if _, mergeErr := d.Copy(ctx, childObj, targetDirObj); mergeErr != nil {
						return nil, fmt.Errorf("copy merge subfolder %s: %w", child.GetName(), mergeErr)
					}
				} else {
					childEncoded := d.encodePath(path.Join(targetPath, child.GetName()))
					childURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, childEncoded)
					if _, fileErr := d.newClient().R().
						SetContext(ctx).
						SetQueryParam("access_token", d.accessToken).
						SetQueryParam("conflict_resolution_strategy", "overwrite").
						SetBody(map[string]interface{}{"copyFrom": childSrcPath}).
						Execute(http.MethodPut, childURL); fileErr != nil {
						return nil, fmt.Errorf("copy merge file %s: %w", child.GetName(), fileErr)
					}
				}
			}

			return &model.Object{
				Name:     srcObj.GetName(),
				Size:     srcObj.GetSize(),
				IsFolder: true,
				Modified: srcObj.ModTime(),
				Ctime:    time.Now(),
				Path:     targetPath,
			}, nil
		}

		// 202 Accepted：polling task status
		if resp.StatusCode() != http.StatusAccepted {
			return nil, fmt.Errorf("unexpected copy folder response: status=%d", resp.StatusCode())
		}

		var taskResp TaskInitResp
		if err := json.Unmarshal(resp.Body(), &taskResp); err != nil || taskResp.TaskId == 0 {
			return nil, fmt.Errorf("failed to parse task id: %w", err)
		}

		taskURL := fmt.Sprintf("%s/task/%s/%s/%d", API_URL, d.libraryId, d.spaceId, taskResp.TaskId)
		actualName := srcObj.GetName()
		taskDone := false

		for i := 0; i < 30 && !taskDone; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(1 * time.Second):
			}

			var taskStatus []TaskStatusItem
			_, pollErr := d.newClient().R().
				SetContext(ctx).
				SetQueryParam("access_token", d.accessToken).
				SetResult(&taskStatus).
				Execute(http.MethodGet, taskURL)

			if pollErr != nil {
				return nil, pollErr
			}
			if len(taskStatus) == 0 {
				continue
			}

			switch taskStatus[0].Status {
			case 200:
				if taskStatus[0].Result != nil && len(taskStatus[0].Result.Path) > 0 {
					actualName = taskStatus[0].Result.Path[len(taskStatus[0].Result.Path)-1]
				}
				taskDone = true
			case 204:
				taskDone = true
			}
		}

		return &model.Object{
			Name:     actualName,
			Size:     srcObj.GetSize(),
			IsFolder: true,
			Modified: srcObj.ModTime(),
			Ctime:    time.Now(),
			Path:     buildPath(dstDir.GetPath(), actualName),
		}, nil
	}
}

func (d *SJTUNetdisk) Remove(ctx context.Context, obj model.Obj) error {
	if err := d.refreshToken(ctx); err != nil {
		return err
	}

	var endpoint string
	if obj.IsDir() {
		endpoint = "directory"
	} else {
		endpoint = "file"
	}

	removeURL := fmt.Sprintf("%s/%s/%s/%s/%s", API_URL, endpoint, d.libraryId, d.spaceId, d.encodePath(obj.GetPath()))
	resp, err := d.newClient().R().
		SetContext(ctx).
		SetQueryParam("access_token", d.accessToken).
		SetQueryParam("permanent", "0").
		SetQueryParam("space_org_id", "1").
		Execute(http.MethodDelete, removeURL)

	if err != nil {
		return err
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return fmt.Errorf("remove failed: status=%d", resp.StatusCode())
	}

	return nil
}

func (d *SJTUNetdisk) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if err := d.refreshToken(ctx); err != nil {
		return nil, err
	}

	parentPath := strings.TrimPrefix(strings.ReplaceAll(dstDir.GetPath(), "\\", "/"), "/")

	// step 1: apply for confirm key
	initURL := fmt.Sprintf("%s/file/%s/%s/%s", API_URL, d.libraryId, d.spaceId, d.encodePath(buildPath(parentPath, stream.GetName())))

	mainClient := d.newClient()

	var initResp UploadInitResp
	_, err := mainClient.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"access_token":                 d.accessToken,
			"user_id":                      d.UserId,
			"conflict_resolution_strategy": "overwrite",
			"filesize":                     strconv.FormatInt(stream.GetSize(), 10),
		}).
		SetBody("{}").
		SetResult(&initResp).
		Execute(http.MethodPut, initURL)

	if err != nil {
		return nil, err
	}

	if initResp.Domain == "" || initResp.Path == "" {
		return nil, err
	}

	// step 2: upload file
	s3URL := fmt.Sprintf("https://%s%s", initResp.Domain, initResp.Path)

	s3Client := resty.New()
	s3Request := s3Client.R().SetContext(ctx)
	for headerKey, headerValue := range initResp.Headers {
		s3Request.SetHeader(headerKey, headerValue)
	}
	s3Request.SetBody(driver.NewLimitedUploadStream(ctx, stream))

	res, err := s3Request.Execute(http.MethodPut, s3URL)

	if err != nil || (res.StatusCode() != http.StatusOK && res.StatusCode() != http.StatusCreated && res.StatusCode() != http.StatusNoContent) {
		return nil, err
	}

	// step 3: confirm the upload
	confirmURL := fmt.Sprintf("%s/file/%s/%s/%s?confirm", API_URL, d.libraryId, d.spaceId, initResp.ConfirmKey)

	var confirmResp ConfirmResp
	_, err = mainClient.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"access_token":                 d.accessToken,
			"user_id":                      d.UserId,
			"conflict_resolution_strategy": "overwrite",
		}).
		SetBody("{}").
		SetResult(&confirmResp).
		Execute(http.MethodPost, confirmURL)

	if err != nil {
		return nil, err
	}

	return &model.Object{
		Name:     stream.GetName(),
		Size:     stream.GetSize(),
		IsFolder: false,
		Modified: time.Now(),
		Ctime:    time.Now(),
		Path:     buildPath(dstDir.GetPath(), stream.GetName()),
	}, nil
}

func (d *SJTUNetdisk) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	return nil, errs.NotSupport
}

var _ driver.Driver = (*SJTUNetdisk)(nil)
var _ driver.Other = (*SJTUNetdisk)(nil)
var _ driver.MkdirResult = (*SJTUNetdisk)(nil)
var _ driver.MoveResult = (*SJTUNetdisk)(nil)
var _ driver.RenameResult = (*SJTUNetdisk)(nil)
var _ driver.PutResult = (*SJTUNetdisk)(nil)
var _ driver.GetRooter = (*SJTUNetdisk)(nil)
