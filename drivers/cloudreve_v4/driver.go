package cloudreve_v4

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type CloudreveV4 struct {
	model.Storage
	Addition
	ref *CloudreveV4
}

func (d *CloudreveV4) Config() driver.Config {
	if d.ref != nil {
		return d.ref.Config()
	}
	if d.EnableVersionUpload {
		config.NoOverwriteUpload = false
	}
	return config
}

func (d *CloudreveV4) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *CloudreveV4) Init(ctx context.Context) error {
	// removing trailing slash
	d.Address = strings.TrimSuffix(d.Address, "/")
	op.MustSaveDriverStorage(d)
	if d.ref != nil {
		return nil
	}
	if d.AccessToken == "" && d.RefreshToken != "" {
		return d.refreshToken()
	}
	if d.Username != "" {
		return d.login()
	}
	return nil
}

func (d *CloudreveV4) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*CloudreveV4)
	if ok {
		d.ref = refStorage
		return nil
	}
	return errs.NotSupport
}

func (d *CloudreveV4) Drop(ctx context.Context) error {
	d.ref = nil
	return nil
}

func (d *CloudreveV4) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	const pageSize int = 100
	var f []File
	var r FileResp
	params := map[string]string{
		"page_size":       strconv.Itoa(pageSize),
		"uri":             dir.GetPath(),
		"order_by":        d.OrderBy,
		"order_direction": d.OrderDirection,
		"page":            "0",
	}

	for {
		err := d.request(http.MethodGet, "/file", func(req *resty.Request) {
			req.SetQueryParams(params)
		}, &r)
		if err != nil {
			return nil, err
		}
		f = append(f, r.Files...)
		if r.Pagination.NextToken == "" || len(r.Files) < pageSize {
			break
		}
		params["next_page_token"] = r.Pagination.NextToken
	}

	return utils.SliceConvert(f, func(src File) (model.Obj, error) {
		if d.EnableFolderSize && src.Type == 1 {
			var ds FolderSummaryResp
			err := d.request(http.MethodGet, "/file/info", func(req *resty.Request) {
				req.SetQueryParam("uri", src.Path)
				req.SetQueryParam("folder_summary", "true")
			}, &ds)
			if err == nil && ds.FolderSummary.Size > 0 {
				src.Size = ds.FolderSummary.Size
			}
		}
		var thumb model.Thumbnail
		if d.EnableThumb && src.Type == 0 {
			var t FileThumbResp
			err := d.request(http.MethodGet, "/file/thumb", func(req *resty.Request) {
				req.SetQueryParam("uri", src.Path)
			}, &t)
			if err == nil && t.URL != "" {
				thumb = model.Thumbnail{
					Thumbnail: t.URL,
				}
			}
		}
		return &model.ObjThumb{
			Object: model.Object{
				ID:       src.ID,
				Path:     src.Path,
				Name:     src.Name,
				Size:     src.Size,
				Modified: src.UpdatedAt,
				Ctime:    src.CreatedAt,
				IsFolder: src.Type == 1,
			},
			Thumbnail: thumb,
		}, nil
	})
}

func (d *CloudreveV4) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var url FileUrlResp
	err := d.request(http.MethodPost, "/file/url", func(req *resty.Request) {
		req.SetBody(base.Json{
			"uris":     []string{file.GetPath()},
			"download": true,
		})
	}, &url)
	if err != nil {
		return nil, err
	}
	if len(url.Urls) == 0 {
		return nil, errors.New("server returns no url")
	}
	exp := time.Until(url.Expires)
	return &model.Link{
		URL:        url.Urls[0].URL,
		Expiration: &exp,
	}, nil
}

func (d *CloudreveV4) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.request(http.MethodPost, "/file/create", func(req *resty.Request) {
		req.SetBody(base.Json{
			"type":              "folder",
			"uri":               parentDir.GetPath() + "/" + dirName,
			"error_on_conflict": true,
		})
	}, nil)
}

func (d *CloudreveV4) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.request(http.MethodPost, "/file/move", func(req *resty.Request) {
		req.SetBody(base.Json{
			"uris": []string{srcObj.GetPath()},
			"dst":  dstDir.GetPath(),
			"copy": false,
		})
	}, nil)
}

func (d *CloudreveV4) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return d.request(http.MethodPost, "/file/create", func(req *resty.Request) {
		req.SetBody(base.Json{
			"new_name": newName,
			"uri":      srcObj.GetPath(),
		})
	}, nil)

}

func (d *CloudreveV4) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.request(http.MethodPost, "/file/move", func(req *resty.Request) {
		req.SetBody(base.Json{
			"uris": []string{srcObj.GetPath()},
			"dst":  dstDir.GetPath(),
			"copy": true,
		})
	}, nil)
}

func (d *CloudreveV4) Remove(ctx context.Context, obj model.Obj) error {
	return d.request(http.MethodDelete, "/file", func(req *resty.Request) {
		req.SetBody(base.Json{
			"uris":             []string{obj.GetPath()},
			"unlink":           false,
			"skip_soft_delete": true,
		})
	}, nil)
}

func (d *CloudreveV4) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	if file.GetSize() == 0 {
		// 空文件使用新建文件方法，避免上传卡锁
		return d.request(http.MethodPost, "/file/create", func(req *resty.Request) {
			req.SetBody(base.Json{
				"type":              "file",
				"uri":               dstDir.GetPath() + "/" + file.GetName(),
				"error_on_conflict": true,
			})
		}, nil)
	}
	var p StoragePolicy
	var r FileResp
	var u FileUploadResp
	var err error
	params := map[string]string{
		"page_size":       "10",
		"uri":             dstDir.GetPath(),
		"order_by":        "created_at",
		"order_direction": "asc",
		"page":            "0",
	}
	err = d.request(http.MethodGet, "/file", func(req *resty.Request) {
		req.SetQueryParams(params)
	}, &r)
	if err != nil {
		return err
	}
	p = r.StoragePolicy
	body := base.Json{
		"uri":           dstDir.GetPath() + "/" + file.GetName(),
		"size":          file.GetSize(),
		"policy_id":     p.ID,
		"last_modified": file.ModTime().UnixMilli(),
		"mime_type":     "",
	}
	if d.EnableVersionUpload {
		body["entity_type"] = "version"
	}
	err = d.request(http.MethodPut, "/file/upload", func(req *resty.Request) {
		req.SetBody(body)
	}, &u)
	if err != nil {
		return err
	}
	if u.StoragePolicy.Relay {
		err = d.upLocal(ctx, file, u, up)
	} else {
		switch u.StoragePolicy.Type {
		case "local":
			err = d.upLocal(ctx, file, u, up)
		case "remote":
			err = d.upRemote(ctx, file, u, up)
		case "onedrive":
			err = d.upOneDrive(ctx, file, u, up)
		case "s3":
			err = d.upS3(ctx, file, u, up)
		default:
			return errs.NotImplement
		}
	}
	if err != nil {
		// 删除失败的会话
		_ = d.request(http.MethodDelete, "/file/upload", func(req *resty.Request) {
			req.SetBody(base.Json{
				"id":  u.SessionID,
				"uri": u.URI,
			})
		}, nil)
		return err
	}
	return nil
}

func (d *CloudreveV4) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *CloudreveV4) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *CloudreveV4) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *CloudreveV4) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *CloudreveV4) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*CloudreveV4)(nil)
