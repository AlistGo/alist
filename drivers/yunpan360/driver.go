package yunpan360

import (
	"context"
	"errors"
	stdpath "path"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type Yunpan360 struct {
	model.Storage
	Addition

	authMu         sync.Mutex
	cachedOpenAuth *OpenAuthInfo
	openAuthExpire time.Time

	cachedCookieSession *CookieDownloadSession
	cookieSessionExpire time.Time
}

func (d *Yunpan360) Config() driver.Config {
	return config
}

func (d *Yunpan360) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Yunpan360) Init(ctx context.Context) error {
	if d.PageSize <= 0 {
		d.PageSize = 100
	}
	d.RootFolderPath = utils.FixAndCleanPath(d.RootFolderPath)
	if d.RootFolderPath == "" {
		d.RootFolderPath = "/"
	}
	d.OrderDirection = strings.ToLower(strings.TrimSpace(d.OrderDirection))
	if d.OrderDirection != "desc" {
		d.OrderDirection = "asc"
	}
	d.AuthType = strings.ToLower(strings.TrimSpace(d.AuthType))
	if d.AuthType == "" {
		d.AuthType = authTypeCookie
	}
	d.SubChannel = strings.TrimSpace(d.SubChannel)
	if d.SubChannel == "" {
		d.SubChannel = defaultSubChannel
	}
	d.EcsEnv = strings.ToLower(strings.TrimSpace(d.EcsEnv))
	if d.EcsEnv == "" {
		d.EcsEnv = openEnvProd
	}
	d.Cookie = strings.TrimSpace(d.Cookie)
	d.APIKey = strings.TrimSpace(d.APIKey)
	d.OwnerQID = strings.TrimSpace(d.OwnerQID)
	d.DownloadToken = strings.TrimSpace(d.DownloadToken)
	d.cachedOpenAuth = nil
	d.openAuthExpire = time.Time{}
	d.cachedCookieSession = nil
	d.cookieSessionExpire = time.Time{}

	switch d.authMode() {
	case authTypeAPIKey:
		if d.APIKey == "" {
			return errors.New("api_key is empty")
		}
		_, err := d.openUserInfo(ctx)
		return err
	case authTypeCookie:
		if d.Cookie == "" {
			return errors.New("cookie is empty")
		}
		// Web download URLs require browser-session headers; force local proxying
		// so AList can forward Referer/Origin instead of exposing a bare 302 URL.
		d.WebProxy = true
		_, err := d.listCookiePage(ctx, d.RootFolderPath, 0, 1)
		return err
	default:
		return errors.New("invalid auth_type")
	}
}

func (d *Yunpan360) Drop(ctx context.Context) error {
	d.cachedOpenAuth = nil
	d.openAuthExpire = time.Time{}
	d.cachedCookieSession = nil
	d.cookieSessionExpire = time.Time{}
	return nil
}

func (d *Yunpan360) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	dirPath := dir.GetPath()
	if dirPath == "" {
		dirPath = d.RootFolderPath
	}

	objs := make([]model.Obj, 0, d.PageSize)
	for page := 0; ; page++ {
		resp, err := d.listPage(ctx, dirPath, page, d.PageSize)
		if err != nil {
			return nil, err
		}
		pageObjs := resp.Objects(dirPath)
		for _, item := range pageObjs {
			objs = append(objs, item)
		}
		if len(pageObjs) == 0 {
			break
		}
		if d.authMode() == authTypeAPIKey {
			if len(pageObjs) < d.PageSize {
				break
			}
			continue
		}
		if !resp.GetHasNextPage() {
			break
		}
	}
	return objs, nil
}

func (d *Yunpan360) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if d.authMode() == authTypeCookie {
		resp, err := d.cookieDownloadURL(ctx, file)
		if err != nil {
			return nil, err
		}
		downloadURL := strings.TrimSpace(resp.GetURL())
		if downloadURL == "" {
			return nil, errors.New("download url is empty")
		}
		return &model.Link{
			URL: downloadURL,
			Header: map[string][]string{
				"Accept":  {"text/javascript, text/html, application/xml, text/xml, */*"},
				"Origin":  {baseURL},
				"Referer": {baseURL + indexPath},
			},
		}, nil
	}
	if d.authMode() != authTypeAPIKey {
		return nil, errs.NotImplement
	}

	resp, err := d.openDownloadURL(ctx, file)
	if err != nil {
		return nil, err
	}
	downloadURL := strings.TrimSpace(resp.GetURL())
	if downloadURL == "" {
		return nil, errors.New("download url is empty")
	}
	return &model.Link{URL: downloadURL}, nil
}

func (d *Yunpan360) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if d.authMode() == authTypeCookie {
		fullPath := ensureDirAPIPath(stdpath.Join(parentDir.GetPath(), dirName))
		resp, err := d.cookieMakeDir(ctx, fullPath)
		if err != nil {
			return nil, err
		}
		return &YunpanObject{
			Object: model.Object{
				ID:       resp.Data.NID,
				Path:     normalizeRemotePath(fullPath),
				Name:     dirName,
				Size:     0,
				Modified: time.Now(),
				IsFolder: true,
			},
		}, nil
	}
	if d.authMode() != authTypeAPIKey {
		return nil, errs.NotImplement
	}

	fullPath := ensureDirAPIPath(stdpath.Join(parentDir.GetPath(), dirName))
	resp, err := d.openMakeDir(ctx, fullPath)
	if err != nil {
		return nil, err
	}
	obj := &model.Object{
		ID:       resp.Data.NID,
		Path:     normalizeRemotePath(fullPath),
		Name:     dirName,
		Size:     0,
		Modified: time.Now(),
		IsFolder: true,
	}
	return obj, nil
}

func (d *Yunpan360) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if d.authMode() == authTypeCookie {
		srcPath := apiPathForObj(srcObj)
		dstPath := ensureDirAPIPath(dstDir.GetPath())
		if err := d.cookieMove(ctx, srcPath, dstPath); err != nil {
			return nil, err
		}
		return cloneObj(srcObj, stdpath.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.GetName()), nil
	}
	if d.authMode() != authTypeAPIKey {
		return nil, errs.NotImplement
	}

	srcPath := apiPathForObj(srcObj)
	dstPath := ensureDirAPIPath(dstDir.GetPath())
	if err := d.openMove(ctx, srcPath, dstPath); err != nil {
		return nil, err
	}
	return cloneObj(srcObj, stdpath.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.GetName()), nil
}

func (d *Yunpan360) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if d.authMode() == authTypeCookie {
		targetName := strings.TrimSuffix(strings.TrimSpace(newName), "/")
		if targetName == "" {
			return nil, errors.New("new name is empty")
		}
		if err := d.cookieRename(ctx, srcObj, targetName); err != nil {
			return nil, err
		}
		parentPath := stdpath.Dir(srcObj.GetPath())
		if parentPath == "." {
			parentPath = "/"
		}
		return cloneObj(srcObj, stdpath.Join(parentPath, targetName), targetName), nil
	}
	if d.authMode() != authTypeAPIKey {
		return nil, errs.NotImplement
	}

	srcPath := apiPathForObj(srcObj)
	targetName := newName
	if srcObj.IsDir() {
		targetName = ensureDirSuffix(newName)
	}
	if err := d.openRename(ctx, srcPath, targetName); err != nil {
		return nil, err
	}

	parentPath := stdpath.Dir(srcObj.GetPath())
	if parentPath == "." {
		parentPath = "/"
	}
	return cloneObj(srcObj, stdpath.Join(parentPath, strings.TrimSuffix(newName, "/")), strings.TrimSuffix(newName, "/")), nil
}

func (d *Yunpan360) Remove(ctx context.Context, obj model.Obj) error {
	if d.authMode() == authTypeCookie {
		return d.cookieRecycle(ctx, obj)
	}
	if d.authMode() != authTypeAPIKey {
		return errs.NotImplement
	}
	return d.openDelete(ctx, apiPathForObj(obj))
}

func (d *Yunpan360) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if d.authMode() == authTypeCookie {
		return nil, errs.NotImplement
	}
	if d.authMode() != authTypeAPIKey {
		return nil, errs.NotImplement
	}
	return d.putOpenFile(ctx, dstDir, file, up)
}

func (d *Yunpan360) authMode() string {
	if d.AuthType == authTypeAPIKey {
		return authTypeAPIKey
	}
	return authTypeCookie
}

var _ driver.Driver = (*Yunpan360)(nil)
