package vtencent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/cron"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type Vtencent struct {
	model.Storage
	Addition
	cron   *cron.Cron
	config driver.Config
	conf   Conf
}

func (d *Vtencent) Config() driver.Config {
	return d.config
}

func (d *Vtencent) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Vtencent) Init(ctx context.Context) error {
	tfUid, err := d.LoadUser()
	if err != nil {
		d.Status = err.Error()
		op.MustSaveDriverStorage(d)
		return nil
	}
	d.Addition.TfUid = tfUid
	op.MustSaveDriverStorage(d)
	d.cron = cron.NewCron(time.Hour * 12)
	d.cron.Do(func() {
		_, err := d.LoadUser()
		if err != nil {
			d.Status = err.Error()
			op.MustSaveDriverStorage(d)
		}
	})
	return nil
}

func (d *Vtencent) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	return nil
}

func (d *Vtencent) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.GetFiles(dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *Vtencent) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	form := fmt.Sprintf(`{"MaterialIds":["%s"]}`, file.GetID())
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(form), &dat); err != nil {
		return nil, err
	}
	var resps RspDown
	api := "https://api.vs.tencent.com/SaaS/Material/DescribeMaterialDownloadUrl"
	rsp, err := d.request(api, http.MethodPost, func(req *resty.Request) {
		req.SetBody(dat)
	}, &resps)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rsp, &resps); err != nil {
		return nil, err
	}
	if len(resps.Data.DownloadURLInfoSet) == 0 {
		return nil, err
	}
	u := resps.Data.DownloadURLInfoSet[0].DownloadURL
	link := &model.Link{
		URL: u,
		Header: http.Header{
			"Referer":    []string{d.conf.referer},
			"User-Agent": []string{d.conf.ua},
		},
		Concurrency: 2,
		PartSize:    10 * utils.MB,
	}
	if file.GetSize() == 0 {
		link.Concurrency = 0
		link.PartSize = 0
	}
	return link, nil
}

func (d *Vtencent) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	classId, err := strconv.Atoi(parentDir.GetID())
	if err != nil {
		return err
	}
	_, err = d.request("https://api.vs.tencent.com/PaaS/Material/CreateClass", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"Owner": base.Json{
				"Type": "PERSON",
				"Id":   d.TfUid,
			},
			"ParentClassId": classId,
			"Name":          dirName,
			"VerifySign":    ""})
	}, nil)
	return err
}

func (d *Vtencent) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcType := "MATERIAL"
	if srcObj.IsDir() {
		srcType = "CLASS"
	}
	form := fmt.Sprintf(`{"SourceInfos":[
		{"Owner":{"Id":"%s","Type":"PERSON"},
		"Resource":{"Type":"%s","Id":"%s"}}
		],
		"Destination":{"Owner":{"Id":"%s","Type":"PERSON"},
		"Resource":{"Type":"CLASS","Id":"%s"}}
		}`, d.TfUid, srcType, srcObj.GetID(), d.TfUid, dstDir.GetID())
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(form), &dat); err != nil {
		return err
	}
	_, err := d.request("https://api.vs.tencent.com/PaaS/Material/MoveResource", http.MethodPost, func(req *resty.Request) {
		req.SetBody(dat)
	}, nil)
	return err
}

func (d *Vtencent) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	api := "https://api.vs.tencent.com/PaaS/Material/ModifyMaterial"
	form := fmt.Sprintf(`{
		"Owner":{"Type":"PERSON","Id":"%s"},
	"MaterialId":"%s","Name":"%s"}`, d.TfUid, srcObj.GetID(), newName)
	if srcObj.IsDir() {
		classId, err := strconv.Atoi(srcObj.GetID())
		if err != nil {
			return err
		}
		api = "https://api.vs.tencent.com/PaaS/Material/ModifyClass"
		form = fmt.Sprintf(`{"Owner":{"Type":"PERSON","Id":"%s"},
	"ClassId":%d,"Name":"%s"}`, d.TfUid, classId, newName)
	}
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(form), &dat); err != nil {
		return err
	}
	_, err := d.request(api, http.MethodPost, func(req *resty.Request) {
		req.SetBody(dat)
	}, nil)
	return err
}

func (d *Vtencent) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	// TODO copy obj, optional
	return errs.NotImplement
}

func (d *Vtencent) Remove(ctx context.Context, obj model.Obj) error {
	srcType := "MATERIAL"
	if obj.IsDir() {
		srcType = "CLASS"
	}
	form := fmt.Sprintf(`{
		"SourceInfos":[
			{"Owner":{"Type":"PERSON","Id":"%s"},
			"Resource":{"Type":"%s","Id":"%s"}}
			]
		}`, d.TfUid, srcType, obj.GetID())
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(form), &dat); err != nil {
		return err
	}
	_, err := d.request("https://api.vs.tencent.com/PaaS/Material/DeleteResource", http.MethodPost, func(req *resty.Request) {
		req.SetBody(dat)
	}, nil)
	return err
}

func (d *Vtencent) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	err := d.FileUpload(ctx, dstDir, stream, up)
	return err
}

//func (d *Vtencent) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Vtencent)(nil)
