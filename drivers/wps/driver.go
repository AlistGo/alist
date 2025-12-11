package wps

import (
	"context"
	"fmt"
	"net/http"

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

func (d *Wps) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	basePath := "/"
	if dir != nil {
		if p := dir.GetPath(); p != "" {
			basePath = p
		}
	}
	node, err := d.resolvePath(ctx, basePath)
	if err != nil {
		return nil, err
	}
	if node.kind == "root" {
		groups, err := d.getGroups(ctx)
		if err != nil {
			return nil, err
		}
		res := make([]model.Obj, 0, len(groups))
		for _, g := range groups {
			path := joinPath(basePath, g.Name)
			obj := &Obj{
				id:    path,
				name:  g.Name,
				ctime: parseTime(0),
				mtime: parseTime(0),
				isDir: true,
				path:  path,
			}
			res = append(res, obj)
		}
		return res, nil
	}
	if node.kind != "group" && node.kind != "folder" {
		return nil, nil
	}
	parentID := int64(0)
	if node.file != nil && node.kind == "folder" {
		parentID = node.file.ID
	}
	files, err := d.getFiles(ctx, node.group.GroupID, parentID)
	if err != nil {
		return nil, err
	}
	res := make([]model.Obj, 0, len(files))
	for _, f := range files {
		res = append(res, fileToObj(basePath, f))
	}
	return res, nil
}

func (d *Wps) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	path := file.GetPath()
	node, err := d.resolvePath(ctx, path)
	if err != nil {
		return nil, err
	}
	if node.kind != "file" || node.file == nil {
		return nil, errs.NotSupport
	}
	if node.file.FilePerms.Download == 0 {
		return nil, fmt.Errorf("no download permission")
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v5/groups/%d/files/%d/download?support_checksums=sha1", endpoint, node.group.GroupID, node.file.ID)
	var resp downloadResp
	_, err = d.request(ctx).SetResult(&resp).Get(url)
	if err != nil {
		return nil, err
	}
	if resp.URL == "" {
		return nil, fmt.Errorf("empty download url")
	}
	link := &model.Link{
		URL:    resp.URL,
		Header: http.Header{},
	}
	return link, nil
}

func (d *Wps) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return errs.NotSupport
}

func (d *Wps) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *Wps) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return errs.NotSupport
}

func (d *Wps) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *Wps) Remove(ctx context.Context, obj model.Obj) error {
	return errs.NotSupport
}

func (d *Wps) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	return errs.NotSupport
}

var _ driver.Driver = (*Wps)(nil)
