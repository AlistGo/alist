package sjtu_netdisk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/go-resty/resty/v2"
)

// refresh and cache access_token, libraryId, spaceId
func (d *SJTUNetdisk) refreshToken(ctx context.Context) error {
	d.tokenMutex.Lock()
	defer d.tokenMutex.Unlock()

	// if token exist and is more than 2 min away from expiration, it will be reused directly
	if d.accessToken != "" && time.Now().Add(2*time.Minute).Before(d.tokenExpires) {
		return nil
	}

	client := d.newClient()
	var resp TokenResp

	_, err := client.R().
		SetContext(ctx).
		SetQueryParam("user_token", d.UserToken).
		SetBody("{}").
		SetResult(&resp).
		Execute(http.MethodPost, TOKEN_URL)

	if err != nil {
		return err
	}

	d.accessToken = resp.AccessToken
	d.libraryId = resp.LibraryId
	d.spaceId = resp.SpaceId

	expiresIn := resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 1800
	}
	d.tokenExpires = time.Now().Add(time.Duration(expiresIn) * time.Second)

	return nil
}

// create a resty client with preloaded USER_TOKEN and keep_alive
func (d *SJTUNetdisk) newClient() *resty.Client {
	client := base.NewRestyClient()
	client.SetCookie(&http.Cookie{Name: "USER_TOKEN", Value: d.UserToken, Path: "/"})
	if d.KeepAlive != "" {
		client.SetCookie(&http.Cookie{Name: "keep_alive", Value: d.KeepAlive, Path: "/"})
	}
	return client
}

// standardize and encode the internal path of alist
func (d *SJTUNetdisk) encodePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "/" || p == "" {
		return ""
	}
	return url.PathEscape(strings.TrimPrefix(p, "/"))
}

// concatenate basic paths and file names, handling root directory boundary situation
func buildPath(base, name string) string {
	if base == "/" || base == "" {
		return "/" + name
	}
	return path.Join(base, name)
}

// move a single file or folder
func (d *SJTUNetdisk) moveItem(ctx context.Context, srcPath, dstDirPath, name string, isDir bool, strategy string) error {
	var endpoint string
	if isDir {
		endpoint = "directory"
	} else {
		endpoint = "file"
	}

	moveItemURL := fmt.Sprintf("%s/%s/%s/%s/%s", API_URL, endpoint, d.libraryId, d.spaceId, d.encodePath(path.Join(dstDirPath, name)))

	req := d.newClient().R().
		SetContext(ctx).
		SetQueryParam("access_token", d.accessToken).
		SetQueryParam("conflict_resolution_strategy", strategy).
		SetBody(map[string]interface{}{"from": srcPath})

	if isDir {
		req.SetQueryParam("move_authority", "true")
	}

	resp, err := req.Execute(http.MethodPut, moveItemURL)
	if err != nil {
		return err
	}

	// 409 means conflict name
	if resp.StatusCode() == http.StatusConflict {
		return errs.ObjectNotFound
	}
	return nil
}
