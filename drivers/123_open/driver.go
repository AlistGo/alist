package _123Open

import (
	"context"
	"fmt"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"net/http"
	"strconv"
	"time"
)

type Open123 struct {
	model.Storage
	Addition

	UploadThread int
	tm           *tokenManager
}

func (d *Open123) Config() driver.Config {
	return config
}

func (d *Open123) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Open123) Init(ctx context.Context) error {
	d.tm = newTokenManager(d.ClientID, d.ClientSecret)

	if _, err := d.tm.getToken(); err != nil {
		return fmt.Errorf("token 初始化失败: %w", err)
	}

	return nil
}

func (d *Open123) Drop(ctx context.Context) error {
	return nil
}

func (d *Open123) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	parentFileId, err := strconv.ParseInt(dir.GetID(), 10, 64)
	if err != nil {
		return nil, err
	}

	fileLastId := int64(0)
	var results []File

	for fileLastId != -1 {
		files, err := d.getFiles(parentFileId, 100, fileLastId)
		if err != nil {
			return nil, err
		}
		for _, f := range files.Data.FileList {
			if f.Trashed == 0 {
				results = append(results, f)
			}
		}
		fileLastId = files.Data.LastFileId
	}

	objs := make([]model.Obj, 0, len(results))
	for _, f := range results {
		objs = append(objs, f)
	}
	return objs, nil
}

func (d *Open123) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.LinkIsDir
	}

	fileID := file.GetID()

	var result DirectLinkResp
	url := fmt.Sprintf("%s?fileID=%s", ApiGetDirectLink, fileID)
	_, err := d.Request(url, http.MethodGet, nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("get link failed: %s", result.Message)
	}

	linkURL := result.Data.URL
	if d.PrivateKey != "" {
		if d.UID == 0 {
			return nil, fmt.Errorf("uid is required when private key is set")
		}
		duration := time.Duration(d.ValidDuration)
		if duration <= 0 {
			duration = 30
		}
		signedURL, err := SignURL(linkURL, d.PrivateKey, d.UID, duration*time.Minute)
		if err != nil {
			return nil, err
		}
		linkURL = signedURL
	}

	return &model.Link{
		URL: linkURL,
	}, nil
}

func (d *Open123) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	parentID, err := strconv.ParseInt(parentDir.GetID(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid parent ID: %w", err)
	}

	var result MakeDirResp
	reqBody := MakeDirRequest{
		Name:     dirName,
		ParentID: parentID,
	}

	_, err = d.Request(ApiMakeDir, http.MethodPost, func(r *resty.Request) {
		r.SetBody(reqBody)
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("mkdir failed: %s", result.Message)
	}

	newDir := File{
		FileId:       result.Data.DirID,
		FileName:     dirName,
		Type:         1,
		ParentFileId: int(parentID),
		Size:         0,
		Trashed:      0,
	}
	return newDir, nil
}

func (d *Open123) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	srcID, err := strconv.ParseInt(srcObj.GetID(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid src file ID: %w", err)
	}
	dstID, err := strconv.ParseInt(dstDir.GetID(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid dest dir ID: %w", err)
	}

	var result Response[any]
	reqBody := map[string]interface{}{
		"fileIDs":        []int64{srcID},
		"toParentFileID": dstID,
	}

	_, err = d.Request(ApiMove, http.MethodPost, func(r *resty.Request) {
		r.SetBody(reqBody)
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("move failed: %s", result.Message)
	}

	files, err := d.getFiles(dstID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("move succeed but failed to get target dir: %w", err)
	}
	for _, f := range files.Data.FileList {
		if f.FileId == srcID {
			return f, nil
		}
	}
	return nil, fmt.Errorf("move succeed but file not found in target dir")
}

func (d *Open123) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	srcID, err := strconv.ParseInt(srcObj.GetID(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid file ID: %w", err)
	}

	var result Response[any]
	reqBody := map[string]interface{}{
		"fileId":   srcID,
		"fileName": newName,
	}

	_, err = d.Request(ApiRename, http.MethodPut, func(r *resty.Request) {
		r.SetBody(reqBody)
	}, &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("rename failed: %s", result.Message)
	}

	parentID := 0
	if file, ok := srcObj.(File); ok {
		parentID = file.ParentFileId
	}
	files, err := d.getFiles(int64(parentID), 100, 0)
	if err != nil {
		return nil, fmt.Errorf("rename succeed but failed to get parent dir: %w", err)
	}
	for _, f := range files.Data.FileList {
		if f.FileId == srcID {
			return f, nil
		}
	}
	return nil, fmt.Errorf("rename succeed but file not found in parent dir")
}

func (d *Open123) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotSupport
}

func (d *Open123) Remove(ctx context.Context, obj model.Obj) error {
	idStr := obj.GetID()
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	var result Response[any]
	reqBody := RemoveRequest{
		FileIDs: []int64{id},
	}

	_, err = d.Request(ApiRemove, http.MethodPost, func(r *resty.Request) {
		r.SetBody(reqBody)
	}, &result)
	if err != nil {
		return err
	}
	if result.Code != 0 {
		return fmt.Errorf("remove failed: %s", result.Message)
	}

	return nil
}

func (d *Open123) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	parentFileId, err := strconv.ParseInt(dstDir.GetID(), 10, 64)
	etag := file.GetHash().GetHash(utils.MD5)

	if len(etag) < utils.MD5.Width {
		up = model.UpdateProgressWithRange(up, 50, 100)
		_, etag, err = stream.CacheFullInTempFileAndHash(file, utils.MD5)
		if err != nil {
			return err
		}
	}
	createResp, err := d.create(parentFileId, file.GetName(), etag, file.GetSize(), 2, false)
	if err != nil {
		return err
	}
	if createResp.Data.Reuse {
		return nil
	}

	return d.Upload(ctx, file, parentFileId, createResp, up)
}

func (d *Open123) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	return nil, errs.NotSupport
}

func (d *Open123) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotSupport
}

func (d *Open123) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	return nil, errs.NotSupport
}

func (d *Open123) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	return nil, errs.NotSupport
}

//func (d *Open123) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Open123)(nil)
