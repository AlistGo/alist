package misskey

import (
	"context"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type Misskey struct {
	model.Storage
	Addition
}

func (d *Misskey) Config() driver.Config {
	return config
}

func (d *Misskey) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Misskey) Init(ctx context.Context) error {
	d.Endpoint = strings.TrimSuffix(d.Endpoint, "/")
	if d.Endpoint == "" || d.AccessToken == "" {
		return errs.EmptyToken
	} else {
		return nil
	}
}

func (d *Misskey) Drop(ctx context.Context) error {
	return nil
}

func (d *Misskey) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return d.list(dir)
}

func (d *Misskey) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	return d.link(file)
}

func (d *Misskey) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	return d.makeDir(parentDir, dirName)
}

func (d *Misskey) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.move(srcObj, dstDir)
}

func (d *Misskey) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	return d.rename(srcObj, newName)
}

func (d *Misskey) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.copy(srcObj, dstDir)
}

func (d *Misskey) Remove(ctx context.Context, obj model.Obj) error {
	return d.remove(obj)
}

func (d *Misskey) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return d.put(ctx, dstDir, stream, up)
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Misskey)(nil)
