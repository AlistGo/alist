package misskey

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

// Base layer methods

func (d *Misskey) request(path, method string, callback base.ReqCallback, resp interface{}) error {
	url := d.Endpoint + "/api/drive" + path
	req := base.RestyClient.R()

	req.SetAuthToken(d.AccessToken).SetHeader("Content-Type", "application/json")

	if callback != nil {
		callback(req)
	} else {
		req.SetBody("{}")
	}

	req.SetResult(resp)

	// 启用调试模式
	req.EnableTrace()

	response, err := req.Execute(method, url)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return errors.New(response.String())
	}
	return nil
}

func (d *Misskey) getThumb(ctx context.Context, obj model.Obj) (io.Reader, error) {
	// TODO return the thumb of obj, optional
	return nil, errs.NotImplement
}

func setBody(body interface{}) base.ReqCallback {
	return func(req *resty.Request) {
		req.SetBody(body)
	}
}

func handleFolderId(dir model.Obj) interface{} {
	if dir.GetID() == "" {
		return nil
	}
	return dir.GetID()
}

// API layer methods

func (d *Misskey) getFiles(dir model.Obj) ([]model.Obj, error) {
	var files []MFile
	var body map[string]string
	if dir.GetPath() != "/" {
		body = map[string]string{"folderId": dir.GetID()}
	} else {
		body = map[string]string{}
	}
	err := d.request("/files", "POST", setBody(body), &files)
	if err != nil {
		return []model.Obj{}, err
	}
	return utils.SliceConvert(files, func(src MFile) (model.Obj, error) {
		return mFile2Object(src), nil
	})
}

func (d *Misskey) getFolders(dir model.Obj) ([]model.Obj, error) {
	var folders []MFolder
	var body map[string]string
	if dir.GetPath() != "/" {
		body = map[string]string{"folderId": dir.GetID()}
	} else {
		body = map[string]string{}
	}
	err := d.request("/folders", "POST", setBody(body), &folders)
	if err != nil {
		return []model.Obj{}, err
	}
	return utils.SliceConvert(folders, func(src MFolder) (model.Obj, error) {
		return mFolder2Object(src), nil
	})
}

func (d *Misskey) list(dir model.Obj) ([]model.Obj, error) {
	files, _ := d.getFiles(dir)
	folders, _ := d.getFolders(dir)
	return append(files, folders...), nil
}

func (d *Misskey) link(file model.Obj) (*model.Link, error) {
	var mFile MFile
	err := d.request("/files/show", "POST", setBody(map[string]string{"fileId": file.GetID()}), &mFile)
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL: mFile.URL,
	}, nil
}

func (d *Misskey) makeDir(parentDir model.Obj, dirName string) (model.Obj, error) {
	var folder MFolder
	err := d.request("/folders/create", "POST", setBody(map[string]interface{}{"parentId": handleFolderId(parentDir), "name": dirName}), &folder)
	if err != nil {
		return nil, err
	}
	return mFolder2Object(folder), nil
}

func (d *Misskey) move(srcObj, dstDir model.Obj) (model.Obj, error) {
	if srcObj.IsDir() {
		var folder MFolder
		err := d.request("/folders/update", "POST", setBody(map[string]interface{}{"folderId": srcObj.GetID(), "parentId": handleFolderId(dstDir)}), &folder)
		return mFolder2Object(folder), err
	} else {
		var file MFile
		err := d.request("/files/update", "POST", setBody(map[string]interface{}{"fileId": srcObj.GetID(), "folderId": handleFolderId(dstDir)}), &file)
		return mFile2Object(file), err
	}
}

func (d *Misskey) rename(srcObj model.Obj, newName string) (model.Obj, error) {
	if srcObj.IsDir() {
		var folder MFolder
		err := d.request("/folders/update", "POST", setBody(map[string]string{"folderId": srcObj.GetID(), "name": newName}), &folder)
		return mFolder2Object(folder), err
	} else {
		var file MFile
		err := d.request("/files/update", "POST", setBody(map[string]string{"fileId": srcObj.GetID(), "name": newName}), &file)
		return mFile2Object(file), err
	}
}

func (d *Misskey) copy(srcObj, dstDir model.Obj) (model.Obj, error) {
	if srcObj.IsDir() {
		folder, err := d.makeDir(dstDir, srcObj.GetName())
		if err != nil {
			return nil, err
		}
		list, err := d.list(srcObj)
		if err != nil {
			return nil, err
		}
		for _, obj := range list {
			_, err := d.copy(obj, folder)
			if err != nil {
				return nil, err
			}
		}
		return folder, nil
	} else {
		var file MFile
		url, err := d.link(srcObj)
		if err != nil {
			return nil, err
		}
		err = d.request("/files/upload-from-url", "POST", setBody(map[string]interface{}{"url": url.URL, "folderId": handleFolderId(dstDir)}), &file)
		if err != nil {
			return nil, err
		}
		return mFile2Object(file), nil
	}
}

func (d *Misskey) remove(obj model.Obj) error {
	if obj.IsDir() {
		err := d.request("/folders/delete", "POST", setBody(map[string]string{"folderId": obj.GetID()}), nil)
		return err
	} else {
		err := d.request("/files/delete", "POST", setBody(map[string]string{"fileId": obj.GetID()}), nil)
		return err
	}
}

func (d *Misskey) put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	var file MFile

	reader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         stream,
		UpdateProgress: up,
	})
	req := base.RestyClient.R().
		SetContext(ctx).
		SetFileReader("file", stream.GetName(), reader).
		SetFormData(map[string]string{
			"folderId":    handleFolderId(dstDir).(string),
			"name":        stream.GetName(),
			"comment":     "",
			"isSensitive": "false",
			"force":       "false",
		}).
		SetResult(&file).
		SetAuthToken(d.AccessToken)

	resp, err := req.Post(d.Endpoint + "/api/drive/files/create")
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, errors.New(resp.String())
	}

	return mFile2Object(file), nil
}

func mFile2Object(file MFile) *model.ObjThumbURL {
	ctime, err := time.Parse(time.RFC3339, file.CreatedAt)
	if err != nil {
		ctime = time.Time{}
	}
	return &model.ObjThumbURL{
		Object: model.Object{
			ID:       file.ID,
			Name:     file.Name,
			Ctime:    ctime,
			IsFolder: false,
			Size:     file.Size,
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: file.ThumbnailURL,
		},
		Url: model.Url{
			Url: file.URL,
		},
	}
}

func mFolder2Object(folder MFolder) *model.Object {
	ctime, err := time.Parse(time.RFC3339, folder.CreatedAt)
	if err != nil {
		ctime = time.Time{}
	}
	return &model.Object{
		ID:       folder.ID,
		Name:     folder.Name,
		Ctime:    ctime,
		IsFolder: true,
	}
}
