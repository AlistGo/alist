package gitee

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type Gitee struct {
	model.Storage
	Addition
	client *resty.Client
}

func (d *Gitee) Config() driver.Config {
	return config
}

func (d *Gitee) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Gitee) Init(ctx context.Context) error {
	d.RootFolderPath = utils.FixAndCleanPath(d.RootFolderPath)
	d.Endpoint = strings.TrimSpace(d.Endpoint)
	if d.Endpoint == "" {
		d.Endpoint = "https://gitee.com/api/v5"
	}
	d.Endpoint = strings.TrimSuffix(d.Endpoint, "/")
	d.Owner = strings.TrimSpace(d.Owner)
	d.Repo = strings.TrimSpace(d.Repo)
	d.Token = strings.TrimSpace(d.Token)
	d.DownloadProxy = strings.TrimSpace(d.DownloadProxy)
	if d.Owner == "" || d.Repo == "" {
		return errors.New("owner and repo are required")
	}
	d.client = base.NewRestyClient().
		SetBaseURL(d.Endpoint).
		SetHeader("Accept", "application/json")
	repo, err := d.getRepo()
	if err != nil {
		return err
	}
	d.Ref = strings.TrimSpace(d.Ref)
	if d.Ref == "" {
		d.Ref = repo.DefaultBranch
	}
	return nil
}

func (d *Gitee) Drop(ctx context.Context) error {
	return nil
}

func (d *Gitee) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	relPath := d.relativePath(dir.GetPath())
	contents, err := d.listContents(relPath)
	if err != nil {
		return nil, err
	}
	objs := make([]model.Obj, 0, len(contents))
	for i := range contents {
		objs = append(objs, contents[i].toModelObj())
	}
	return objs, nil
}

func (d *Gitee) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var downloadURL string
	if obj, ok := file.(*Object); ok {
		downloadURL = obj.DownloadURL
		if downloadURL == "" {
			relPath := d.relativePath(file.GetPath())
			content, err := d.getContent(relPath)
			if err != nil {
				return nil, err
			}
			if content.DownloadURL == "" {
				return nil, errors.New("empty download url")
			}
			obj.DownloadURL = content.DownloadURL
			downloadURL = content.DownloadURL
		}
	} else {
		relPath := d.relativePath(file.GetPath())
		content, err := d.getContent(relPath)
		if err != nil {
			return nil, err
		}
		if content.DownloadURL == "" {
			return nil, errors.New("empty download url")
		}
		downloadURL = content.DownloadURL
	}
	url := d.applyProxy(downloadURL)
	return &model.Link{
		URL: url,
		Header: http.Header{
			"Cookie": {d.Cookie},
		},
	}, nil
}

func (d *Gitee) newRequest() *resty.Request {
	req := d.client.R()
	if d.Token != "" {
		req.SetQueryParam("access_token", d.Token)
	}
	if d.Ref != "" {
		req.SetQueryParam("ref", d.Ref)
	}
	return req
}

func (d *Gitee) apiPath(path string) string {
	escapedOwner := url.PathEscape(d.Owner)
	escapedRepo := url.PathEscape(d.Repo)
	if path == "" {
		return fmt.Sprintf("/repos/%s/%s/contents", escapedOwner, escapedRepo)
	}
	return fmt.Sprintf("/repos/%s/%s/contents/%s", escapedOwner, escapedRepo, encodePath(path))
}

func (d *Gitee) listContents(path string) ([]Content, error) {
	res, err := d.newRequest().Get(d.apiPath(path))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var contents []Content
	if err := utils.Json.Unmarshal(res.Body(), &contents); err != nil {
		var single Content
		if err2 := utils.Json.Unmarshal(res.Body(), &single); err2 == nil && single.Type != "" {
			if single.Type != "dir" {
				return nil, errs.NotFolder
			}
			return []Content{}, nil
		}
		return nil, err
	}
	for i := range contents {
		contents[i].Path = joinPath(path, contents[i].Name)
	}
	return contents, nil
}

func (d *Gitee) getContent(path string) (*Content, error) {
	res, err := d.newRequest().Get(d.apiPath(path))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var content Content
	if err := utils.Json.Unmarshal(res.Body(), &content); err != nil {
		return nil, err
	}
	if content.Type == "" {
		return nil, errors.New("invalid response")
	}
	if content.Path == "" {
		content.Path = path
	}
	return &content, nil
}

func (d *Gitee) relativePath(full string) string {
	full = utils.FixAndCleanPath(full)
	root := utils.FixAndCleanPath(d.RootFolderPath)
	if root == "/" {
		return strings.TrimPrefix(full, "/")
	}
	if utils.PathEqual(full, root) {
		return ""
	}
	prefix := utils.PathAddSeparatorSuffix(root)
	if strings.HasPrefix(full, prefix) {
		return strings.TrimPrefix(full, prefix)
	}
	return strings.TrimPrefix(full, "/")
}

func (d *Gitee) applyProxy(raw string) string {
	if raw == "" || d.DownloadProxy == "" {
		return raw
	}
	proxy := d.DownloadProxy
	if !strings.HasSuffix(proxy, "/") {
		proxy += "/"
	}
	return proxy + strings.TrimLeft(raw, "/")
}

func encodePath(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func joinPath(base, name string) string {
	if base == "" {
		return name
	}
	return strings.TrimPrefix(stdpath.Join(base, name), "./")
}
