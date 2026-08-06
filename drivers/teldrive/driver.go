package teldrive

import (
	"context"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"math"
	"net/http"
	"net/url"
	"strings"
)

type Teldrive struct {
	model.Storage
	Addition
}

func (d *Teldrive) Config() driver.Config {
	return config
}

func (d *Teldrive) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Teldrive) Init(ctx context.Context) error {
	// TODO login / refresh token
	// op.MustSaveDriverStorage(d)
	if d.Cookie == "" || !strings.HasPrefix(d.Cookie, "access_token=") {
		return fmt.Errorf("cookie must start with 'access_token='")
	}
	if d.UploadConcurrency == 0 {
		d.UploadConcurrency = 4
	}
	if d.ChunkSize == 0 {
		d.ChunkSize = 10
	}
	if d.WebdavNative() {
		d.WebProxy = true
	} else {
		d.WebProxy = false
	}

	op.MustSaveDriverStorage(d)
	return nil
}

func (d *Teldrive) Drop(ctx context.Context) error {
	return nil
}

func (d *Teldrive) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	// TODO return the files list, required
	// endpoint /api/files， params ->page order sort path
	var listResp ListResp
	params := url.Values{}
	params.Set("path", dir.GetPath())
	//log.Info(dir.GetPath())
	pathname, err := utils.InjectQuery("/api/files", params)
	if err != nil {
		return nil, err
	}

	err = d.request(http.MethodGet, pathname, nil, &listResp)
	if err != nil {
		return nil, err
	}

	return utils.SliceConvert(listResp.Items, func(src Object) (model.Obj, error) {
		return &model.Object{
			ID:   src.ID,
			Name: src.Name,
			Size: func() int64 {
				if src.Type == "folder" {
					return 0
				}
				return src.Size
			}(),
			IsFolder: src.Type == "folder",
			Modified: src.UpdatedAt,
		}, nil
	})
}

func (d *Teldrive) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if !d.WebdavNative() {
		if shareObj, err := d.getShareFileById(file.GetID()); err == nil && shareObj != nil {
			return &model.Link{
				URL: d.Address + fmt.Sprintf("/api/shares/%s/files/%s/%s", shareObj.Id, file.GetID(), file.GetName()),
			}, nil
		}
		if err := d.createShareFile(file.GetID()); err != nil {
			return nil, err
		}
		shareObj, err := d.getShareFileById(file.GetID())
		if err != nil {
			return nil, err
		}
		return &model.Link{
			URL: d.Address + fmt.Sprintf("/api/shares/%s/files/%s/%s", shareObj.Id, file.GetID(), file.GetName()),
		}, nil
	}
	return &model.Link{
		URL: d.Address + "/api/files/" + file.GetID() + "/" + file.GetName(),
		Header: http.Header{
			"Cookie": {d.Cookie},
		},
	}, nil
}

func (d *Teldrive) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.request(http.MethodPost, "/api/files/mkdir", func(req *resty.Request) {
		req.SetBody(map[string]interface{}{
			"path": parentDir.GetPath() + "/" + dirName,
		})
	}, nil)
}

func (d *Teldrive) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	body := base.Json{
		"ids":               []string{srcObj.GetID()},
		"destinationParent": dstDir.GetID(),
	}
	return d.request(http.MethodPost, "/api/files/move", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Teldrive) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	body := base.Json{
		"name": newName,
	}
	return d.request(http.MethodPatch, "/api/files/"+srcObj.GetID(), func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Teldrive) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	copyConcurrentLimit := 4
	copyManager := NewCopyManager(ctx, copyConcurrentLimit, d)
	copyManager.startWorkers()
	copyManager.G.Go(func() error {
		defer close(copyManager.TaskChan)
		return copyManager.generateTasks(ctx, srcObj, dstDir)
	})
	return copyManager.G.Wait()
}

func (d *Teldrive) Remove(ctx context.Context, obj model.Obj) error {
	body := base.Json{
		"ids": []string{obj.GetID()},
	}
	return d.request(http.MethodPost, "/api/files/delete", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Teldrive) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	fileId := uuid.New().String()
	chunkSizeInMB := d.ChunkSize
	chunkSize := chunkSizeInMB * 1024 * 1024 // Convert MB to bytes
	totalSize := file.GetSize()
	totalParts := int(math.Ceil(float64(totalSize) / float64(chunkSize)))
	retryCount := 0
	maxRetried := 3
	p := driver.NewProgress(totalSize, up)

	// delete the upload task when finished or failed
	defer func() {
		_ = d.request(http.MethodDelete, "/api/uploads/"+fileId, nil, nil)
	}()

	if obj, err := d.getFile(dstDir.GetPath(), file.GetName(), file.IsDir()); err == nil {
		if err = d.Remove(ctx, obj); err != nil {
			return err
		}
	}
	// start the upload process
	if err := d.request(http.MethodGet, "/api/uploads/"+fileId, nil, nil); err != nil {
		return err
	}
	if totalSize == 0 {
		return d.touch(file.GetName(), dstDir.GetPath())
	}

	if totalParts <= 1 {
		return d.doSingleUpload(ctx, dstDir, file, p, retryCount, maxRetried, totalParts, fileId)
	}

	return d.doMultiUpload(ctx, dstDir, file, p, maxRetried, totalParts, chunkSize, fileId)
}

func (d *Teldrive) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Teldrive) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Teldrive) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Teldrive) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *Teldrive) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Teldrive)(nil)
