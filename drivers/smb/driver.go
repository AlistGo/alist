package smb

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"

	"github.com/hirochachacha/go-smb2"
)

type SMB struct {
	lastConnTime int64
	model.Storage
	Addition
	fs *smb2.Share
}

func (d *SMB) Config() driver.Config {
	return config
}

func (d *SMB) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *SMB) Init(ctx context.Context) error {
	if strings.Index(d.Addition.Address, ":") < 0 {
		d.Addition.Address = d.Addition.Address + ":445"
	}
	return d.initFS()
}

func (d *SMB) Drop(ctx context.Context) error {
	if d.fs != nil {
		_ = d.fs.Umount()
	}
	return nil
}

func (d *SMB) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if err := d.checkConn(); err != nil {
		return nil, err
	}
	fullPath := dir.GetPath()
	rawFiles, err := d.fs.ReadDir(fullPath)
	if err != nil {
		d.cleanLastConnTime()
		return nil, err
	}
	d.updateLastConnTime()
	var files []model.Obj
	for _, f := range rawFiles {
		file := model.ObjThumb{
			Object: model.Object{
				Name:     f.Name(),
				Modified: f.ModTime(),
				Size:     f.Size(),
				IsFolder: f.IsDir(),
				Ctime:    f.(*smb2.FileStat).CreationTime,
			},
		}
		files = append(files, &file)
	}
	return files, nil
}

func (d *SMB) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := d.checkConn(); err != nil {
		return nil, err
	}
	fullPath := file.GetPath()
	remoteFile, err := d.fs.Open(fullPath)
	if err != nil {
		d.cleanLastConnTime()
		return nil, err
	}
	link := &model.Link{
		MFile: remoteFile,
	}
	d.updateLastConnTime()
	return link, nil
}

func (d *SMB) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	fullPath := filepath.Join(parentDir.GetPath(), dirName)
	err := d.fs.MkdirAll(fullPath, 0700)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	err := d.fs.Rename(srcPath, dstPath)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(filepath.Dir(srcPath), newName)
	err := d.fs.Rename(srcPath, dstPath)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	var err error
	if srcObj.IsDir() {
		err = d.CopyDir(srcPath, dstPath)
	} else {
		err = d.CopyFile(srcPath, dstPath)
	}
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Remove(ctx context.Context, obj model.Obj) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	var err error
	fullPath := obj.GetPath()
	if obj.IsDir() {
		err = d.fs.RemoveAll(fullPath)
	} else {
		err = d.fs.Remove(fullPath)
	}
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	return nil
}

func (d *SMB) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	if err := d.checkConn(); err != nil {
		return err
	}
	fullPath := filepath.Join(dstDir.GetPath(), stream.GetName())
	out, err := d.fs.Create(fullPath)
	if err != nil {
		d.cleanLastConnTime()
		return err
	}
	d.updateLastConnTime()
	defer func() {
		_ = out.Close()
		if errors.Is(err, context.Canceled) {
			_ = d.fs.Remove(fullPath)
		}
	}()
	err = utils.CopyWithCtx(ctx, out, driver.NewLimitedUploadStream(ctx, stream), stream.GetSize(), up)
	if err != nil {
		return err
	}
	return nil
}

//func (d *SMB) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*SMB)(nil)
