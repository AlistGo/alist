package kodbox

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type KodBox struct {
	model.Storage
	Addition
	authorization string
}

func (d *KodBox) Config() driver.Config {
	return config
}

func (d *KodBox) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *KodBox) Init(ctx context.Context) error {
	d.Address = strings.TrimSuffix(d.Address, "/")
	d.RootFolderPath = strings.TrimPrefix(utils.FixAndCleanPath(d.RootFolderPath), "/")
	return d.getToken()
}

func (d *KodBox) Drop(ctx context.Context) error {
	return nil
}

func (d *KodBox) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var (
		resp         *CommonResp
		listPathData *ListPathData
	)

	_, err := d.request(http.MethodPost, "/?explorer/list/path", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"path": dir.GetPath(),
		})
	}, true)
	if err != nil {
		return nil, err
	}

	dataBytes, err := utils.Json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	err = utils.Json.Unmarshal(dataBytes, &listPathData)
	if err != nil {
		return nil, err
	}
	FolderAndFiles := append(listPathData.FolderList, listPathData.FileList...)

	return utils.SliceConvert(FolderAndFiles, func(f FolderOrFile) (model.Obj, error) {
		return &model.ObjThumb{
			Object: model.Object{
				Path:     f.Path,
				Name:     f.Name,
				Ctime:    time.Unix(f.CreateTime, 0),
				Modified: time.Unix(f.ModifyTime, 0),
				Size:     f.Size,
				IsFolder: f.Type == "folder",
			},
			//Thumbnail: model.Thumbnail{},
		}, nil
	})
}

func (d *KodBox) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	path := file.GetPath()
	return &model.Link{
		URL: fmt.Sprintf("%s/?explorer/index/fileOut&path=%s&download=1&accessToken=%s",
			d.Address,
			path,
			d.authorization)}, nil
}

func (d *KodBox) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	var resp *CommonResp
	newDirPath := filepath.Join(parentDir.GetPath(), dirName)

	_, err := d.request(http.MethodPost, "/?explorer/index/mkdir", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"path": newDirPath,
		})
	})
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}

	return &model.ObjThumb{
		Object: model.Object{
			Path:     resp.Info.(string),
			Name:     dirName,
			IsFolder: true,
			Modified: time.Now(),
			Ctime:    time.Now(),
		},
	}, nil
}

func (d *KodBox) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/index/pathCuteTo", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"dataArr": fmt.Sprintf("[{\"path\": \"%s\", \"name\": \"%s\"}]",
				srcObj.GetPath(),
				srcObj.GetName()),
			"path": dstDir.GetPath(),
		})
	}, true)
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}

	return &model.ObjThumb{
		Object: model.Object{
			Path:     srcObj.GetPath(),
			Name:     srcObj.GetName(),
			IsFolder: srcObj.IsDir(),
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
		},
	}, nil
}

func (d *KodBox) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/index/pathRename", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"path":    srcObj.GetPath(),
			"newName": newName,
		})
	}, true)
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}
	return &model.ObjThumb{
		Object: model.Object{
			Path:     srcObj.GetPath(),
			Name:     newName,
			IsFolder: srcObj.IsDir(),
			Modified: time.Now(),
			Ctime:    srcObj.CreateTime(),
		},
	}, nil
}

func (d *KodBox) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/index/pathCopyTo", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"dataArr": fmt.Sprintf("[{\"path\": \"%s\", \"name\": \"%s\"}]",
				srcObj.GetPath(),
				srcObj.GetName()),
			"path": dstDir.GetPath(),
		})
	})
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}

	path := resp.Info.([]interface{})[0].(string)
	objectName, err := d.getFileOrFolderName(ctx, path)
	if err != nil {
		return nil, err
	}
	return &model.ObjThumb{
		Object: model.Object{
			Path:     path,
			Name:     *objectName,
			IsFolder: srcObj.IsDir(),
			Modified: time.Now(),
			Ctime:    time.Now(),
		},
	}, nil
}

func (d *KodBox) Remove(ctx context.Context, obj model.Obj) error {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/index/pathDelete", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"dataArr": fmt.Sprintf("[{\"path\": \"%s\", \"name\": \"%s\"}]",
				obj.GetPath(),
				obj.GetName()),
			"shiftDelete": "1",
		})
	})
	if err != nil {
		return err
	}
	code := resp.Code.(bool)
	if !code {
		return fmt.Errorf("%s", resp.Data)
	}
	return nil
}

func (d *KodBox) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/upload/fileUpload", func(req *resty.Request) {
		r := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
			Reader:         s,
			UpdateProgress: up,
		})
		req.SetFileReader("file", s.GetName(), r).
			SetResult(&resp).
			SetFormData(map[string]string{
				"path": dstDir.GetPath(),
			}).
			SetContext(ctx)
	})
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}
	return &model.ObjThumb{
		Object: model.Object{
			Path:     resp.Info.(string),
			Name:     s.GetName(),
			Size:     s.GetSize(),
			IsFolder: false,
			Modified: time.Now(),
			Ctime:    time.Now(),
		},
	}, nil
}

func (d *KodBox) getFileOrFolderName(ctx context.Context, path string) (*string, error) {
	var resp *CommonResp
	_, err := d.request(http.MethodPost, "/?explorer/index/pathInfo", func(req *resty.Request) {
		req.SetResult(&resp).SetFormData(map[string]string{
			"dataArr": fmt.Sprintf("[{\"path\": \"%s\"}]", path)})
	})
	if err != nil {
		return nil, err
	}
	code := resp.Code.(bool)
	if !code {
		return nil, fmt.Errorf("%s", resp.Data)
	}
	folderOrFileName := resp.Data.(map[string]any)["name"].(string)
	return &folderOrFileName, nil
}

var _ driver.Driver = (*KodBox)(nil)
