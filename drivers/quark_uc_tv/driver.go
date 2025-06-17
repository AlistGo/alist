package quark_uc_tv

import (
	"context"
	"fmt"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type QuarkUCTV struct {
	*QuarkUCTVCommon
	model.Storage
	Addition
	config driver.Config
	conf   Conf
}

func (d *QuarkUCTV) Config() driver.Config {
	return d.config
}

func (d *QuarkUCTV) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *QuarkUCTV) Init(ctx context.Context) error {

	if d.Addition.DeviceID == "" {
		d.Addition.DeviceID = utils.GetMD5EncodeStr(time.Now().String())
	}
	op.MustSaveDriverStorage(d)

	if d.QuarkUCTVCommon == nil {
		d.QuarkUCTVCommon = &QuarkUCTVCommon{
			AccessToken: "",
		}
	}
	ctx1, cancelFunc := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFunc()
	if d.Addition.RefreshToken == "" {
		if d.Addition.QueryToken == "" {
			qrData, err := d.getLoginCode(ctx1)
			if err != nil {
				return err
			}
			// 展示二维码
			qrTemplate := `<body>
        <img src="data:image/jpeg;base64,%s"/>
    </body>`
			qrPage := fmt.Sprintf(qrTemplate, qrData)
			return fmt.Errorf("need verify: \n%s", qrPage)
		} else {
			// 通过query token获取code -> refresh token
			code, err := d.getCode(ctx1)
			if err != nil {
				return err
			}
			// 通过code获取refresh token
			err = d.getRefreshTokenByTV(ctx1, code, false)
			if err != nil {
				return err
			}
		}
	}
	// 通过refresh token获取access token
	if d.QuarkUCTVCommon.AccessToken == "" {
		err := d.getRefreshTokenByTV(ctx1, d.Addition.RefreshToken, true)
		if err != nil {
			return err
		}
	}

	// 验证 access token 是否有效
	_, err := d.isLogin(ctx1)
	if err != nil {
		return err
	}
	return nil
}

func (d *QuarkUCTV) Drop(ctx context.Context) error {
	return nil
}

func (d *QuarkUCTV) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	pageIndex := int64(0)
	pageSize := int64(100)
	for {
		var filesData FilesData
		_, err := d.request(ctx, "/file", "GET", func(req *resty.Request) {
			req.SetQueryParams(map[string]string{
				"method":     "list",
				"parent_fid": dir.GetID(),
				"order_by":   "3",
				"desc":       "1",
				"category":   "",
				"source":     "",
				"ex_source":  "",
				"list_all":   "0",
				"page_size":  strconv.FormatInt(pageSize, 10),
				"page_index": strconv.FormatInt(pageIndex, 10),
			})
		}, &filesData)
		if err != nil {
			return nil, err
		}
		for i := range filesData.Data.Files {
			files = append(files, &filesData.Data.Files[i])
		}
		if pageIndex*pageSize >= filesData.Data.TotalCount {
			break
		} else {
			pageIndex++
		}
	}
	return files, nil
}

func (d *QuarkUCTV) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var fileLink FileLink
	_, err := d.request(ctx, "/file", "GET", func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"method":     "download",
			"group_by":   "source",
			"fid":        file.GetID(),
			"resolution": "low,normal,high,super,2k,4k",
			"support":    "dolby_vision",
		})
	}, &fileLink)
	if err != nil {
		return nil, err
	}
	
	return &model.Link{
		URL:  fileLink.Data.DownloadURL,
		Concurrency: 3,
		PartSize:    10 * utils.MB,
	}, nil
}

func (d *QuarkUCTV) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *QuarkUCTV) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *QuarkUCTV) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *QuarkUCTV) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *QuarkUCTV) Remove(ctx context.Context, obj model.Obj) error {
	return errs.NotImplement
}

func (d *QuarkUCTV) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return nil, errs.NotImplement
}

type QuarkUCTVCommon struct {
	AccessToken string
}

var _ driver.Driver = (*QuarkUCTV)(nil)
