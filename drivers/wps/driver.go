package wps

import (
	"context"
	"fmt"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type Wps struct {
	model.Storage
	Addition
	companyID string
}

func (d *Wps) Config() driver.Config {
	return config
}

func (d *Wps) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Wps) Init(ctx context.Context) error {
	if d.Cookie == "" {
		return fmt.Errorf("cookie is empty")
	}
	return d.ensureCompanyID(ctx)
}

func (d *Wps) Drop(ctx context.Context) error {
	return nil
}

func (d *Wps) List(ctx context.Context, dir model.Obj, _ model.ListArgs) ([]model.Obj, error) {
	basePath := "/"
	if dir != nil {
		if p := dir.GetPath(); p != "" {
			basePath = p
		}
	}
	return d.list(ctx, basePath)
}

func (d *Wps) Link(ctx context.Context, file model.Obj, _ model.LinkArgs) (*model.Link, error) {
	if file == nil {
		return nil, errs.NotSupport
	}
	return d.link(ctx, file.GetPath())
}

func (d *Wps) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.makeDir(ctx, parentDir, dirName)
}

func (d *Wps) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.move(ctx, srcObj, dstDir)
}

func (d *Wps) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return d.rename(ctx, srcObj, newName)
}

func (d *Wps) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.copy(ctx, srcObj, dstDir)
}

func (d *Wps) Remove(ctx context.Context, obj model.Obj) error {
	return d.remove(ctx, obj)
}

func (d *Wps) Put(ctx context.Context, _ model.Obj, _ model.FileStreamer, _ driver.UpdateProgress) error {
	return errs.NotSupport
}

var _ driver.Driver = (*Wps)(nil)
