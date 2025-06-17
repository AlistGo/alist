package ipfs

import (
	"context"
	"fmt"
	"net/url"
	"path"

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
	var ipfsPath string
	cid := dir.GetID()
	if cid != "" {
		ipfsPath = path.Join("/ipfs", cid)
	} else {
		// 可能出现ipns dns解析失败的情况，需要重复获取cid，其他情况应该不会出错
		ipfsPath = dir.GetPath()
		switch d.Mode {
		case "ipfs":
			ipfsPath = path.Join("/ipfs", ipfsPath)
		case "ipns":
			ipfsPath = path.Join("/ipns", ipfsPath)
		case "mfs":
			fileStat, err := d.sh.FilesStat(ctx, ipfsPath)
			if err != nil {
				return nil, err
			}
			ipfsPath = path.Join("/ipfs", fileStat.Hash)
		default:
			return nil, fmt.Errorf("mode error")
		}
	}
	dirs, err := d.sh.List(ipfsPath)
	if err != nil {
		return nil, err
	}

	objlist := []model.Obj{}
	for _, file := range dirs {
		objlist = append(objlist, &model.Object{ID: file.Hash, Name: file.Name, Size: int64(file.Size), IsFolder: file.Type == 1})
	}

	return objlist, nil
}

func (d *IPFS) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	gateurl := d.gateURL.JoinPath("/ipfs/", file.GetID())
	gateurl.RawQuery = "filename=" + url.QueryEscape(file.GetName())
	return &model.Link{URL: gateurl.String()}, nil
}

func (d *IPFS) Get(ctx context.Context, rawPath string) (model.Obj, error) {
	rawPath = path.Join(d.GetRootPath(), rawPath)
	var ipfsPath string
	switch d.Mode {
	case "ipfs":
		ipfsPath = path.Join("/ipfs", rawPath)
	case "ipns":
		ipfsPath = path.Join("/ipns", rawPath)
	case "mfs":
		fileStat, err := d.sh.FilesStat(ctx, rawPath)
		if err != nil {
			return nil, err
		}
		ipfsPath = path.Join("/ipfs", fileStat.Hash)
	default:
		return nil, fmt.Errorf("mode error")
	}
	file, err := d.sh.FilesStat(ctx, ipfsPath)
	if err != nil {
		return nil, err
	}
	return &model.Object{ID: file.Hash, Name: path.Base(rawPath), Path: rawPath, Size: int64(file.Size), IsFolder: file.Type == "directory"}, nil
}

func (d *IPFS) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if d.Mode != "mfs" {
		return nil, fmt.Errorf("only write in mfs mode")
	}
	dirPath := parentDir.GetPath()
	err := d.sh.FilesMkdir(ctx, path.Join(dirPath, dirName), shell.FilesMkdir.Parents(true))
	if err != nil {
		return nil, err
	}
	file, err := d.sh.FilesStat(ctx, path.Join(dirPath, dirName))
	if err != nil {
		return nil, err
	}
	return &model.Object{ID: file.Hash, Name: dirName, Path: path.Join(dirPath, dirName), Size: int64(file.Size), IsFolder: true}, nil
}

func (d *IPFS) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if d.Mode != "mfs" {
		return nil, fmt.Errorf("only write in mfs mode")
	}
	dstPath := path.Join(dstDir.GetPath(), path.Base(srcObj.GetPath()))
	d.sh.FilesRm(ctx, dstPath, true)
	return &model.Object{ID: srcObj.GetID(), Name: srcObj.GetName(), Path: dstPath, Size: int64(srcObj.GetSize()), IsFolder: srcObj.IsDir()},
		d.sh.FilesMv(ctx, srcObj.GetPath(), dstDir.GetPath())
}

func (d *IPFS) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if d.Mode != "mfs" {
		return nil, fmt.Errorf("only write in mfs mode")
	}
	dstPath := path.Join(path.Dir(srcObj.GetPath()), newName)
	d.sh.FilesRm(ctx, dstPath, true)
	return &model.Object{ID: srcObj.GetID(), Name: newName, Path: dstPath, Size: int64(srcObj.GetSize()),
		IsFolder: srcObj.IsDir()}, d.sh.FilesMv(ctx, srcObj.GetPath(), dstPath)
}

func (d *IPFS) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if d.Mode != "mfs" {
		return nil, fmt.Errorf("only write in mfs mode")
	}
	dstPath := path.Join(dstDir.GetPath(), path.Base(srcObj.GetPath()))
	d.sh.FilesRm(ctx, dstPath, true)
	return &model.Object{ID: srcObj.GetID(), Name: srcObj.GetName(), Path: dstPath, Size: int64(srcObj.GetSize()), IsFolder: srcObj.IsDir()},
		d.sh.FilesCp(ctx, path.Join("/ipfs/", srcObj.GetID()), dstPath, shell.FilesCp.Parents(true))
}

func (d *IPFS) Remove(ctx context.Context, obj model.Obj) error {
	if d.Mode != "mfs" {
		return fmt.Errorf("only write in mfs mode")
	}
	return d.sh.FilesRm(ctx, obj.GetPath(), true)
}

func (d *IPFS) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if d.Mode != "mfs" {
		return nil, fmt.Errorf("only write in mfs mode")
	}
	outHash, err := d.sh.Add(driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	}))
	if err != nil {
		return nil, err
	}
	dstPath := path.Join(dstDir.GetPath(), s.GetName())
	if s.GetExist() != nil {
		d.sh.FilesRm(ctx, dstPath, true)
	}
	err = d.sh.FilesCp(ctx, path.Join("/ipfs/", outHash), dstPath, shell.FilesCp.Parents(true))
	gateurl := d.gateURL.JoinPath("/ipfs/", outHash)
	gateurl.RawQuery = "filename=" + url.QueryEscape(s.GetName())
	return &model.Object{ID: outHash, Name: s.GetName(), Path: dstPath, Size: int64(s.GetSize()), IsFolder: s.IsDir()}, err
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*IPFS)(nil)
