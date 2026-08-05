package emby

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
)

const authHeader = `MediaBrowser Client="AList", Device="AList", DeviceId="AList", Version="3.0.0"`

func (d *Emby) baseURL() string {
	return strings.TrimRight(d.Address, "/")
}

func (d *Emby) login(ctx context.Context) error {
	var resp AuthResp
	res, err := base.RestyClient.R().
		SetContext(ctx).
		SetHeader("X-Emby-Authorization", authHeader).
		SetBody(map[string]string{
			"Username": d.Username,
			"Pw":       d.Password,
		}).
		SetResult(&resp).
		Post(d.baseURL() + "/Users/AuthenticateByName")
	if err != nil {
		return err
	}
	if res.StatusCode() != http.StatusOK {
		return fmt.Errorf("emby login failed: %s: %s", res.Status(), res.String())
	}
	if resp.AccessToken == "" || resp.User.Id == "" {
		return fmt.Errorf("emby login failed: empty token or user id in response")
	}
	d.token = resp.AccessToken
	d.userID = resp.User.Id
	d.canDownload = resp.User.Policy.EnableContentDownloading
	return nil
}

func (d *Emby) ensureLogin(ctx context.Context) error {
	d.loginMutex.Lock()
	defer d.loginMutex.Unlock()
	if d.token != "" {
		return nil
	}
	return d.login(ctx)
}

func (d *Emby) relogin(ctx context.Context) error {
	d.loginMutex.Lock()
	defer d.loginMutex.Unlock()
	return d.login(ctx)
}

// request sends an authenticated GET request, re-logging in once on 401
func (d *Emby) request(ctx context.Context, path string, query map[string]string, out interface{}) error {
	if err := d.ensureLogin(ctx); err != nil {
		return err
	}
	do := func() (*resty.Response, error) {
		return base.RestyClient.R().
			SetContext(ctx).
			SetHeader("X-Emby-Token", d.token).
			SetQueryParams(query).
			SetResult(out).
			Get(d.baseURL() + path)
	}
	res, err := do()
	if err != nil {
		return err
	}
	if res.StatusCode() == http.StatusUnauthorized {
		if err = d.relogin(ctx); err != nil {
			return err
		}
		res, err = do()
		if err != nil {
			return err
		}
	}
	if res.StatusCode() != http.StatusOK {
		return fmt.Errorf("emby request failed: %s: %s", res.Status(), res.String())
	}
	return nil
}

// toObj converts an emby item to an alist object
func (d *Emby) toObj(item Item) model.Obj {
	name := item.Name
	size := item.Size
	if !item.IsFolder {
		// prefer the real filename so the extension is kept for players
		if item.Path != "" {
			name = path.Base(strings.ReplaceAll(item.Path, "\\", "/"))
		} else if len(item.MediaSources) > 0 && item.MediaSources[0].Path != "" {
			name = path.Base(strings.ReplaceAll(item.MediaSources[0].Path, "\\", "/"))
		} else if item.Container != "" && !strings.Contains(item.Container, ",") {
			name = item.Name + "." + item.Container
		}
		if size == 0 && len(item.MediaSources) > 0 {
			size = item.MediaSources[0].Size
		}
	}
	name = strings.ReplaceAll(name, "/", "／")
	obj := model.ObjThumb{
		Object: model.Object{
			ID:       item.Id,
			Name:     name,
			Size:     size,
			Modified: item.DateCreated,
			Ctime:    item.DateCreated,
			IsFolder: item.IsFolder,
		},
	}
	if tag, ok := item.ImageTags["Primary"]; ok && tag != "" {
		obj.Thumbnail.Thumbnail = fmt.Sprintf("%s/Items/%s/Images/Primary?maxWidth=400&quality=90&tag=%s",
			d.baseURL(), item.Id, tag)
	}
	return &obj
}
