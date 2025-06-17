package trainbit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type Trainbit struct {
	model.Storage
	Addition
}

var apiExpiredate, guid string

func (d *Trainbit) Config() driver.Config {
	return config
}

func (d *Trainbit) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Trainbit) Init(ctx context.Context) error {
	base.HttpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	var err error
	apiExpiredate, guid, err = getToken(d.ApiKey, d.AUSHELLPORTAL)
	if err != nil {
		return err
	}
	return nil
}

func (d *Trainbit) Drop(ctx context.Context) error {
	return nil
}

func (d *Trainbit) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	form := make(url.Values)
	form.Set("parentid", strings.Split(dir.GetID(), "_")[0])
	res, err := postForm("https://trainbit.com/lib/api/v1/listoffiles", form, apiExpiredate, d.ApiKey, d.AUSHELLPORTAL)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var jsonData any
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, err
	}
	object, err := parseRawFileObject(jsonData.(map[string]any)["items"].([]any))
	if err != nil {
		return nil, err
	}
	return object, nil
}

func (d *Trainbit) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	res, err := get(fmt.Sprintf("https://trainbit.com/files/%s/", strings.Split(file.GetID(), "_")[0]), d.ApiKey, d.AUSHELLPORTAL)
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL: res.Header.Get("Location"),
	}, nil
}

func (d *Trainbit) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	form := make(url.Values)
	form.Set("name", local2provider(dirName, true))
	form.Set("parentid", strings.Split(parentDir.GetID(), "_")[0])
	_, err := postForm("https://trainbit.com/lib/api/v1/createfolder", form, apiExpiredate, d.ApiKey, d.AUSHELLPORTAL)
	return err
}

func (d *Trainbit) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	form := make(url.Values)
	form.Set("sourceid", strings.Split(srcObj.GetID(), "_")[0])
	form.Set("destinationid", strings.Split(dstDir.GetID(), "_")[0])
	_, err := postForm("https://trainbit.com/lib/api/v1/move", form, apiExpiredate, d.ApiKey, d.AUSHELLPORTAL)
	return err
}

func (d *Trainbit) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	form := make(url.Values)
	form.Set("id", strings.Split(srcObj.GetID(), "_")[0])
	form.Set("name", local2provider(newName, srcObj.IsDir()))
	_, err := postForm("https://trainbit.com/lib/api/v1/edit", form, apiExpiredate, d.ApiKey, d.AUSHELLPORTAL)
	return err
}

func (d *Trainbit) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotImplement
}

func (d *Trainbit) Remove(ctx context.Context, obj model.Obj) error {
	form := make(url.Values)
	form.Set("id", strings.Split(obj.GetID(), "_")[0])
	_, err := postForm("https://trainbit.com/lib/api/v1/delete", form, apiExpiredate, d.ApiKey, d.AUSHELLPORTAL)
	return err
}

func (d *Trainbit) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	endpoint, _ := url.Parse("https://tb28.trainbit.com/api/upload/send_raw/")
	query := &url.Values{}
	query.Add("q", strings.Split(dstDir.GetID(), "_")[1])
	query.Add("guid", guid)
	query.Add("name", url.QueryEscape(local2provider(s.GetName(), false)+"."))
	endpoint.RawQuery = query.Encode()
	progressReader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         s,
		UpdateProgress: up,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), progressReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/json; charset=UTF-8")
	_, err = base.HttpClient.Do(req)
	return err
}

var _ driver.Driver = (*Trainbit)(nil)
