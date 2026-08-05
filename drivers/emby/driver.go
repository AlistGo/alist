package emby

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

type Emby struct {
	model.Storage
	Addition

	token       string
	userID      string
	canDownload bool
	loginMutex  sync.Mutex
}

func (d *Emby) Config() driver.Config {
	return config
}

func (d *Emby) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Emby) Init(ctx context.Context) error {
	return d.relogin(ctx)
}

func (d *Emby) Drop(ctx context.Context) error {
	return nil
}

func (d *Emby) GetRoot(ctx context.Context) (model.Obj, error) {
	return &model.Object{
		ID:       d.RootFolderID,
		Path:     "/",
		Name:     "root",
		IsFolder: true,
	}, nil
}

func (d *Emby) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if err := d.ensureLogin(ctx); err != nil {
		return nil, err
	}

	// root without a root_folder_id: list library views
	if dir.GetID() == "" {
		var resp ItemsResp
		if err := d.request(ctx, "/Users/"+d.userID+"/Views", nil, &resp); err != nil {
			return nil, err
		}
		objs := make([]model.Obj, 0, len(resp.Items))
		for _, item := range resp.Items {
			objs = append(objs, d.toObj(item))
		}
		return objs, nil
	}

	const pageSize = 1000
	var objs []model.Obj
	for start := 0; ; start += pageSize {
		var resp ItemsResp
		err := d.request(ctx, "/Users/"+d.userID+"/Items", map[string]string{
			"ParentId":   dir.GetID(),
			"Fields":     "DateCreated,Path,MediaSources",
			"StartIndex": strconv.Itoa(start),
			"Limit":      strconv.Itoa(pageSize),
		}, &resp)
		if err != nil {
			return nil, err
		}
		for _, item := range resp.Items {
			objs = append(objs, d.toObj(item))
		}
		if start+len(resp.Items) >= resp.TotalRecordCount || len(resp.Items) == 0 {
			break
		}
	}
	return objs, nil
}

func (d *Emby) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if err := d.ensureLogin(ctx); err != nil {
		return nil, err
	}
	if d.canDownload {
		return &model.Link{
			URL: fmt.Sprintf("%s/Items/%s/Download?api_key=%s", d.baseURL(), file.GetID(), d.token),
		}, nil
	}
	// no download permission: fall back to the direct-play stream of the original file
	var resp ItemsResp
	err := d.request(ctx, "/Users/"+d.userID+"/Items", map[string]string{"Ids": file.GetID()}, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Items) > 0 {
		switch resp.Items[0].MediaType {
		case "Video":
			return &model.Link{
				URL: fmt.Sprintf("%s/Videos/%s/stream?static=true&api_key=%s", d.baseURL(), file.GetID(), d.token),
			}, nil
		case "Audio":
			return &model.Link{
				URL: fmt.Sprintf("%s/Audio/%s/stream?static=true&api_key=%s", d.baseURL(), file.GetID(), d.token),
			}, nil
		}
	}
	return nil, fmt.Errorf("emby user has no download permission and item is not streamable")
}

var _ driver.Driver = (*Emby)(nil)
var _ driver.GetRooter = (*Emby)(nil)
