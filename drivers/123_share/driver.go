package _123Share

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	_123 "github.com/alist-org/alist/v3/drivers/123"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type Pan123Share struct {
	model.Storage
	Addition
	apiRateLimit sync.Map
	ref          *_123.Pan123
}

func (d *Pan123Share) Config() driver.Config {
	return config
}

func (d *Pan123Share) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Pan123Share) Init(ctx context.Context) error {
	// TODO login / refresh token
	//op.MustSaveDriverStorage(d)
	return nil
}

func (d *Pan123Share) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*_123.Pan123)
	if ok {
		d.ref = refStorage
		return nil
	}
	return fmt.Errorf("ref: storage is not 123Pan")
}

func (d *Pan123Share) Drop(ctx context.Context) error {
	d.ref = nil
	return nil
}

func (d *Pan123Share) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	// TODO return the files list, required
	files, err := d.getFiles(ctx, dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return src, nil
	})
}

func (d *Pan123Share) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	// TODO return link of file, required
	if f, ok := file.(File); ok {
		//var resp DownResp
		var headers map[string]string
		if !utils.IsLocalIPAddr(args.IP) {
			headers = map[string]string{
				//"X-Real-IP":       "1.1.1.1",
				"X-Forwarded-For": args.IP,
			}
		}
		data := base.Json{
			"shareKey":  d.ShareKey,
			"SharePwd":  d.SharePwd,
			"etag":      f.Etag,
			"fileId":    f.FileId,
			"s3keyFlag": f.S3KeyFlag,
			"size":      f.Size,
		}
		resp, err := d.request(DownloadInfo, http.MethodPost, func(req *resty.Request) {
			req.SetBody(data).SetHeaders(headers)
		}, nil)
		if err != nil {
			return nil, err
		}
		downloadUrl := utils.Json.Get(resp, "data", "DownloadURL").ToString()
		u, err := url.Parse(downloadUrl)
		if err != nil {
			return nil, err
		}
		nu := u.Query().Get("params")
		if nu != "" {
			du, _ := base64.StdEncoding.DecodeString(nu)
			u, err = url.Parse(string(du))
			if err != nil {
				return nil, err
			}
		}
		u_ := u.String()
		log.Debug("download url: ", u_)
		res, err := base.NoRedirectClient.R().SetHeader("Referer", "https://www.123pan.com/").Get(u_)
		if err != nil {
			return nil, err
		}
		log.Debug(res.String())
		link := model.Link{
			URL: u_,
		}
		log.Debugln("res code: ", res.StatusCode())
		if res.StatusCode() == 302 {
			link.URL = res.Header().Get("location")
		} else if res.StatusCode() < 300 {
			link.URL = utils.Json.Get(res.Body(), "data", "redirect_url").ToString()
		}
		link.Header = http.Header{
			"Referer": []string{"https://www.123pan.com/"},
		}
		return &link, nil
	}
	return nil, fmt.Errorf("can't convert obj")
}

func (d *Pan123Share) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	// TODO create folder, optional
	return errs.NotSupport
}

func (d *Pan123Share) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	// TODO move obj, optional
	return errs.NotSupport
}

func (d *Pan123Share) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	// TODO rename obj, optional
	return errs.NotSupport
}

func (d *Pan123Share) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	// TODO copy obj, optional
	return errs.NotSupport
}

func (d *Pan123Share) Remove(ctx context.Context, obj model.Obj) error {
	// TODO remove obj, optional
	return errs.NotSupport
}

func (d *Pan123Share) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	// TODO upload file, optional
	return errs.NotSupport
}

//func (d *Pan123Share) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

func (d *Pan123Share) APIRateLimit(ctx context.Context, api string) error {
	value, _ := d.apiRateLimit.LoadOrStore(api,
		rate.NewLimiter(rate.Every(700*time.Millisecond), 1))
	limiter := value.(*rate.Limiter)

	return limiter.Wait(ctx)
}

var _ driver.Driver = (*Pan123Share)(nil)
