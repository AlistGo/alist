package yandex_disk

import (
	"context"
	"net/http"
	"path"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type YandexDisk struct {
	model.Storage
	Addition
	AccessToken string
}

func (d *YandexDisk) Config() driver.Config {
	return config
}

func (d *YandexDisk) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *YandexDisk) Init(ctx context.Context) error {
	return d.refreshToken()
}

func (d *YandexDisk) Drop(ctx context.Context) error {
	return nil
}

func (d *YandexDisk) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(dir.GetPath())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *YandexDisk) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var resp DownResp
	_, err := d.request("/download", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("path", file.GetPath())
	}, &resp)
	if err != nil {
		return nil, err
	}
	link := model.Link{
		URL: resp.Href,
	}
	return &link, nil
}

func (d *YandexDisk) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	_, err := d.request("", http.MethodPut, func(req *resty.Request) {
		req.SetQueryParam("path", path.Join(parentDir.GetPath(), dirName))
	}, nil)
	return err
}

func (d *YandexDisk) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := d.request("/move", http.MethodPost, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"from":      srcObj.GetPath(),
			"path":      path.Join(dstDir.GetPath(), srcObj.GetName()),
			"overwrite": "true",
		})
	}, nil)
	return err
}

func (d *YandexDisk) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	_, err := d.request("/move", http.MethodPost, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"from":      srcObj.GetPath(),
			"path":      path.Join(path.Dir(srcObj.GetPath()), newName),
			"overwrite": "true",
		})
	}, nil)
	return err
}

func (d *YandexDisk) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := d.request("/copy", http.MethodPost, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"from":      srcObj.GetPath(),
			"path":      path.Join(dstDir.GetPath(), srcObj.GetName()),
			"overwrite": "true",
		})
	}, nil)
	return err
}

func (d *YandexDisk) Remove(ctx context.Context, obj model.Obj) error {
	_, err := d.request("", http.MethodDelete, func(req *resty.Request) {
		req.SetQueryParam("path", obj.GetPath())
	}, nil)
	return err
}

func (d *YandexDisk) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	var resp UploadResp
	_, err := d.request("/upload", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"path":      path.Join(dstDir.GetPath(), s.GetName()),
			"overwrite": "true",
		})
	}, &resp)
	if err != nil {
		return err
	}
	reader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	})
	req, err := http.NewRequestWithContext(ctx, resp.Method, resp.Href, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Length", strconv.FormatInt(s.GetSize(), 10))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := base.HttpClient.Do(req)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	return err
}

var _ driver.Driver = (*YandexDisk)(nil)
