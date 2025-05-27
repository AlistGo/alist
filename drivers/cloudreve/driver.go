package cloudreve

import (
	"context"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type Cloudreve struct {
	model.Storage
	Addition
	ref *Cloudreve
}

func (d *Cloudreve) Config() driver.Config {
	return config
}

func (d *Cloudreve) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Cloudreve) Init(ctx context.Context) error {
	if d.Cookie != "" {
		return nil
	}
	// removing trailing slash
	d.Address = strings.TrimSuffix(d.Address, "/")
	return d.login()
}

func (d *Cloudreve) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*Cloudreve)
	if ok {
		d.ref = refStorage
		return nil
	}
	return errs.NotSupport
}

func (d *Cloudreve) Drop(ctx context.Context) error {
	d.Cookie = ""
	d.ref = nil
	return nil
}

func (d *Cloudreve) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var r DirectoryResp
	err := d.request(http.MethodGet, "/directory"+dir.GetPath(), nil, &r)
	if err != nil {
		return nil, err
	}

	return utils.SliceConvert(r.Objects, func(src Object) (model.Obj, error) {
		thumb, err := d.GetThumb(src)
		if err != nil {
			return nil, err
		}
		if src.Type == "dir" && d.EnableThumbAndFolderSize {
			var dprop DirectoryProp
			err = d.request(http.MethodGet, "/object/property/"+src.Id+"?is_folder=true", nil, &dprop)
			if err != nil {
				return nil, err
			}
			src.Size = dprop.Size
		}
		return objectToObj(src, thumb), nil
	})
}

func (d *Cloudreve) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var dUrl string
	err := d.request(http.MethodPut, "/file/download/"+file.GetID(), nil, &dUrl)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(dUrl, "/api") {
		dUrl = d.Address + dUrl
	}
	return &model.Link{
		URL: dUrl,
	}, nil
}

func (d *Cloudreve) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.request(http.MethodPut, "/directory", func(req *resty.Request) {
		req.SetBody(base.Json{
			"path": parentDir.GetPath() + "/" + dirName,
		})
	}, nil)
}

func (d *Cloudreve) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	body := base.Json{
		"action":  "move",
		"src_dir": path.Dir(srcObj.GetPath()),
		"dst":     dstDir.GetPath(),
		"src":     convertSrc(srcObj),
	}
	return d.request(http.MethodPatch, "/object", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Cloudreve) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	body := base.Json{
		"action":   "rename",
		"new_name": newName,
		"src":      convertSrc(srcObj),
	}
	return d.request(http.MethodPatch, "/object/rename", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Cloudreve) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	body := base.Json{
		"src_dir": path.Dir(srcObj.GetPath()),
		"dst":     dstDir.GetPath(),
		"src":     convertSrc(srcObj),
	}
	return d.request(http.MethodPost, "/object/copy", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

func (d *Cloudreve) Remove(ctx context.Context, obj model.Obj) error {
	body := convertSrc(obj)
	err := d.request(http.MethodDelete, "/object", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
	return err
}

func (d *Cloudreve) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	if io.ReadCloser(stream) == http.NoBody {
		return d.create(ctx, dstDir, stream)
	}

	// 获取存储策略
	var r DirectoryResp
	err := d.request(http.MethodGet, "/directory"+dstDir.GetPath(), nil, &r)
	if err != nil {
		return err
	}
	uploadBody := base.Json{
		"path":          dstDir.GetPath(),
		"size":          stream.GetSize(),
		"name":          stream.GetName(),
		"policy_id":     r.Policy.Id,
		"last_modified": stream.ModTime().UnixMilli(),
	}

	// 获取上传会话信息
	var u UploadInfo
	err = d.request(http.MethodPut, "/file/upload", func(req *resty.Request) {
		req.SetBody(uploadBody)
	}, &u)
	if err != nil {
		return err
	}

	// 根据存储方式选择分片上传的方法
	switch r.Policy.Type {
	case "onedrive":
		err = d.upOneDrive(ctx, stream, u, up)
	case "s3":
		err = d.upS3(ctx, stream, u, up)
	case "remote": // 从机存储
		err = d.upRemote(ctx, stream, u, up)
	case "local": // 本机存储
		err = d.upLocal(ctx, stream, u, up)
	default:
		err = errs.NotImplement
	}
	if err != nil {
		// 删除失败的会话
		_ = d.request(http.MethodDelete, "/file/upload/"+u.SessionID, nil, nil)
		return err
	}
	return nil
}

func (d *Cloudreve) create(ctx context.Context, dir model.Obj, file model.Obj) error {
	body := base.Json{"path": dir.GetPath() + "/" + file.GetName()}
	if file.IsDir() {
		err := d.request(http.MethodPut, "directory", func(req *resty.Request) {
			req.SetBody(body)
		}, nil)
		return err
	}
	return d.request(http.MethodPost, "/file/create", func(req *resty.Request) {
		req.SetBody(body)
	}, nil)
}

//func (d *Cloudreve) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Cloudreve)(nil)
