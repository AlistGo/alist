package ipfs

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	shell "github.com/ipfs/go-ipfs-api"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

type IPFS struct {
	model.Storage
	Addition
	sh      *shell.Shell
	gateURL *url.URL
}

func (d *IPFS) Config() driver.Config {
	return config
}

func (d *IPFS) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *IPFS) Init(ctx context.Context) error {
	d.sh = shell.NewShell(d.Endpoint)
	gateURL, err := url.Parse(d.Gateway)
	if err != nil {
		return err
	}
	d.gateURL = gateURL
	return nil
}

func (d *IPFS) Drop(ctx context.Context) error {
	return nil
}

func (d *IPFS) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	ipfsPath := dir.GetPath()
	switch d.Mode {
	case "ipfs":
		ipfsPath = filepath.Join("/ipfs", ipfsPath)
	case "ipns":
		ipfsPath = filepath.Join("/ipns", ipfsPath)
	case "mfs":
		fileStat, err := d.sh.FilesStat(ctx, ipfsPath)
		if err != nil {
			return nil, err
		}
		ipfsPath = filepath.Join("/ipfs", fileStat.Hash)
	default:
		return nil, fmt.Errorf("mode error")
	}
	dirs, err := d.sh.List(ipfsPath)
	if err != nil {
		return nil, err
	}

	objlist := []model.Obj{}
	for _, file := range dirs {
		gateurl := *d.gateURL.JoinPath("/ipfs/", file.Hash)
		gateurl.RawQuery = "filename=" + url.QueryEscape(file.Name)
		objlist = append(objlist, &model.ObjectURL{
			Object: model.Object{ID: file.Hash, Name: file.Name, Size: int64(file.Size), IsFolder: file.Type == 1},
			Url:    model.Url{Url: gateurl.String()},
		})
	}

	return objlist, nil
}

func (d *IPFS) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	gateurl := d.gateURL.JoinPath("/ipfs/", file.GetID())
	gateurl.RawQuery = "filename=" + url.QueryEscape(file.GetName())
	return &model.Link{URL: gateurl.String()}, nil
}

func (d *IPFS) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	path := parentDir.GetPath()
	return d.sh.FilesMkdir(ctx, filepath.Join(path, dirName))
}

func (d *IPFS) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	return d.sh.FilesMv(ctx, srcObj.GetPath(), dstDir.GetPath())
}

func (d *IPFS) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	dstPath := filepath.Join(filepath.Dir(srcObj.GetPath()), newName)
	return d.sh.FilesMv(ctx, srcObj.GetPath(), dstPath)
}

func (d *IPFS) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	dstPath := filepath.Join(dstDir.GetPath(), filepath.Base(srcObj.GetPath()))
	return d.sh.FilesCp(ctx, srcObj.GetPath(), dstPath)
}

func (d *IPFS) Remove(ctx context.Context, obj model.Obj) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	return d.sh.FilesRm(ctx, obj.GetPath(), true)
}

func (d *IPFS) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	outHash, err := d.sh.Add(driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	}))
	if err != nil {
		return err
	}
	err = d.sh.FilesCp(ctx, filepath.Join("/ipfs/", outHash), filepath.Join(dstDir.GetPath(), s.GetName()))
	return err
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*IPFS)(nil)
