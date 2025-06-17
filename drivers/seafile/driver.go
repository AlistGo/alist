package seafile

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type Seafile struct {
	model.Storage
	Addition

	authorization string
	libraryMap    map[string]*LibraryInfo
}

func (d *Seafile) Config() driver.Config {
	return config
}

func (d *Seafile) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Seafile) Init(ctx context.Context) error {
	d.Address = strings.TrimSuffix(d.Address, "/")
	d.RootFolderPath = utils.FixAndCleanPath(d.RootFolderPath)
	d.libraryMap = make(map[string]*LibraryInfo)
	return d.getToken()
}

func (d *Seafile) Drop(ctx context.Context) error {
	return nil
}

func (d *Seafile) List(ctx context.Context, dir model.Obj, args model.ListArgs) (result []model.Obj, err error) {
	path := dir.GetPath()
	if path == d.RootFolderPath {
		libraries, err := d.listLibraries()
		if err != nil {
			return nil, err
		}
		if path == "/" && d.RepoId == "" {
			return utils.SliceConvert(libraries, func(f LibraryItemResp) (model.Obj, error) {
				return &model.Object{
					Name:     f.Name,
					Modified: time.Unix(f.Modified, 0),
					Size:     f.Size,
					IsFolder: true,
				}, nil
			})
		}
	}
	var repo *LibraryInfo
	repo, path, err = d.getRepoAndPath(path)
	if err != nil {
		return nil, err
	}
	if repo.Encrypted {
		err = d.decryptLibrary(repo)
		if err != nil {
			return nil, err
		}
	}
	var resp []RepoDirItemResp
	_, err = d.request(http.MethodGet, fmt.Sprintf("/api2/repos/%s/dir/", repo.Id), func(req *resty.Request) {
		req.SetResult(&resp).SetQueryParams(map[string]string{
			"p": path,
		})
	})
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(resp, func(f RepoDirItemResp) (model.Obj, error) {
		return &model.ObjThumb{
			Object: model.Object{
				Name:     f.Name,
				Modified: time.Unix(f.Modified, 0),
				Size:     f.Size,
				IsFolder: f.Type == "dir",
			},
			// Thumbnail: model.Thumbnail{Thumbnail: f.Thumb},
		}, nil
	})
}

func (d *Seafile) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	repo, path, err := d.getRepoAndPath(file.GetPath())
	if err != nil {
		return nil, err
	}
	res, err := d.request(http.MethodGet, fmt.Sprintf("/api2/repos/%s/file/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p":     path,
			"reuse": "1",
		})
	})
	if err != nil {
		return nil, err
	}
	u := string(res)
	u = u[1 : len(u)-1] // remove quotes
	return &model.Link{URL: u}, nil
}

func (d *Seafile) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	repo, path, err := d.getRepoAndPath(parentDir.GetPath())
	if err != nil {
		return err
	}
	path, _ = utils.JoinBasePath(path, dirName)
	_, err = d.request(http.MethodPost, fmt.Sprintf("/api2/repos/%s/dir/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		}).SetFormData(map[string]string{
			"operation": "mkdir",
		})
	})
	return err
}

func (d *Seafile) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	repo, path, err := d.getRepoAndPath(srcObj.GetPath())
	if err != nil {
		return err
	}
	dstRepo, dstPath, err := d.getRepoAndPath(dstDir.GetPath())
	if err != nil {
		return err
	}
	_, err = d.request(http.MethodPost, fmt.Sprintf("/api2/repos/%s/file/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		}).SetFormData(map[string]string{
			"operation": "move",
			"dst_repo":  dstRepo.Id,
			"dst_dir":   dstPath,
		})
	}, true)
	return err
}

func (d *Seafile) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	repo, path, err := d.getRepoAndPath(srcObj.GetPath())
	if err != nil {
		return err
	}
	_, err = d.request(http.MethodPost, fmt.Sprintf("/api2/repos/%s/file/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		}).SetFormData(map[string]string{
			"operation": "rename",
			"newname":   newName,
		})
	}, true)
	return err
}

func (d *Seafile) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	repo, path, err := d.getRepoAndPath(srcObj.GetPath())
	if err != nil {
		return err
	}
	dstRepo, dstPath, err := d.getRepoAndPath(dstDir.GetPath())
	if err != nil {
		return err
	}
	_, err = d.request(http.MethodPost, fmt.Sprintf("/api2/repos/%s/file/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		}).SetFormData(map[string]string{
			"operation": "copy",
			"dst_repo":  dstRepo.Id,
			"dst_dir":   dstPath,
		})
	})
	return err
}

func (d *Seafile) Remove(ctx context.Context, obj model.Obj) error {
	repo, path, err := d.getRepoAndPath(obj.GetPath())
	if err != nil {
		return err
	}
	_, err = d.request(http.MethodDelete, fmt.Sprintf("/api2/repos/%s/file/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		})
	})
	return err
}

func (d *Seafile) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	repo, path, err := d.getRepoAndPath(dstDir.GetPath())
	if err != nil {
		return err
	}
	res, err := d.request(http.MethodGet, fmt.Sprintf("/api2/repos/%s/upload-link/", repo.Id), func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"p": path,
		})
	})
	if err != nil {
		return err
	}

	u := string(res)
	u = u[1 : len(u)-1] // remove quotes
	_, err = d.request(http.MethodPost, u, func(req *resty.Request) {
		r := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
			Reader:         s,
			UpdateProgress: up,
		})
		req.SetFileReader("file", s.GetName(), r).
			SetFormData(map[string]string{
				"parent_dir": path,
				"replace":    "1",
			}).
			SetContext(ctx)
	})
	return err
}

var _ driver.Driver = (*Seafile)(nil)
