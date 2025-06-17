package webdav

import (
	"context"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/cron"
	"github.com/alist-org/alist/v3/pkg/gowebdav"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type WebDav struct {
	model.Storage
	Addition
	client *gowebdav.Client
	cron   *cron.Cron
}

func (d *WebDav) Config() driver.Config {
	return config
}

func (d *WebDav) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *WebDav) Init(ctx context.Context) error {
	err := d.setClient()
	if err == nil {
		d.cron = cron.NewCron(time.Hour * 12)
		d.cron.Do(func() {
			_ = d.setClient()
		})
	}
	return err
}

func (d *WebDav) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	return nil
}

func (d *WebDav) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.client.ReadDir(dir.GetPath())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src os.FileInfo) (model.Obj, error) {
		return &model.Object{
			Name:     src.Name(),
			Size:     src.Size(),
			Modified: src.ModTime(),
			IsFolder: src.IsDir(),
		}, nil
	})
}

func (d *WebDav) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	url, header, err := d.client.Link(file.GetPath())
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL:    url,
		Header: header,
	}, nil
}

func (d *WebDav) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.client.MkdirAll(path.Join(parentDir.GetPath(), dirName), 0644)
}

func (d *WebDav) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.client.Rename(getPath(srcObj), path.Join(dstDir.GetPath(), srcObj.GetName()), true)
}

func (d *WebDav) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return d.client.Rename(getPath(srcObj), path.Join(path.Dir(srcObj.GetPath()), newName), true)
}

func (d *WebDav) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.client.Copy(getPath(srcObj), path.Join(dstDir.GetPath(), srcObj.GetName()), true)
}

func (d *WebDav) Remove(ctx context.Context, obj model.Obj) error {
	return d.client.RemoveAll(getPath(obj))
}

func (d *WebDav) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	callback := func(r *http.Request) {
		r.Header.Set("Content-Type", s.GetMimetype())
		r.ContentLength = s.GetSize()
	}
	reader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	})
	err := d.client.WriteStream(path.Join(dstDir.GetPath(), s.GetName()), reader, 0644, callback)
	return err
}

var _ driver.Driver = (*WebDav)(nil)
