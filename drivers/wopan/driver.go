package template

import (
	"context"
	"fmt"
	"strconv"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/xhofe/wopan-sdk-go"
)

type Wopan struct {
	model.Storage
	Addition
	client          *wopan.WoClient
	defaultFamilyID string
}

func (d *Wopan) Config() driver.Config {
	return config
}

func (d *Wopan) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Wopan) Init(ctx context.Context) error {
	d.client = wopan.DefaultWithRefreshToken(d.RefreshToken)
	d.client.SetAccessToken(d.AccessToken)
	d.client.OnRefreshToken(func(accessToken, refreshToken string) {
		d.AccessToken = accessToken
		d.RefreshToken = refreshToken
		op.MustSaveDriverStorage(d)
	})
	fml, err := d.client.FamilyUserCurrentEncode()
	if err != nil {
		return err
	}
	d.defaultFamilyID = strconv.Itoa(fml.DefaultHomeId)
	return d.client.InitData()
}

func (d *Wopan) Drop(ctx context.Context) error {
	return nil
}

func (d *Wopan) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var res []model.Obj
	pageNum := 0
	pageSize := 100
	for {
		data, err := d.client.QueryAllFiles(d.getSpaceType(), dir.GetID(), pageNum, pageSize, 0, d.FamilyID, func(req *resty.Request) {
			req.SetContext(ctx)
		})
		if err != nil {
			return nil, err
		}
		objs, err := utils.SliceConvert(data.Files, fileToObj)
		if err != nil {
			return nil, err
		}
		res = append(res, objs...)
		if len(data.Files) < pageSize {
			break
		}
		pageNum++
	}
	return res, nil
}

func (d *Wopan) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if f, ok := file.(*Object); ok {
		res, err := d.client.GetDownloadUrlV2([]string{f.FID}, func(req *resty.Request) {
			req.SetContext(ctx)
		})
		if err != nil {
			return nil, err
		}
		return &model.Link{
			URL: res.List[0].DownloadUrl,
		}, nil
	}
	return nil, fmt.Errorf("unable to convert file to Object")
}

func (d *Wopan) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	familyID := d.FamilyID
	if familyID == "" {
		familyID = d.defaultFamilyID
	}
	_, err := d.client.CreateDirectory(d.getSpaceType(), parentDir.GetID(), dirName, familyID, func(req *resty.Request) {
		req.SetContext(ctx)
	})
	return err
}

func (d *Wopan) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	dirList := make([]string, 0)
	fileList := make([]string, 0)
	if srcObj.IsDir() {
		dirList = append(dirList, srcObj.GetID())
	} else {
		fileList = append(fileList, srcObj.GetID())
	}
	return d.client.MoveFile(dirList, fileList, dstDir.GetID(),
		d.getSpaceType(), d.getSpaceType(),
		d.FamilyID, d.FamilyID, func(req *resty.Request) {
			req.SetContext(ctx)
		})
}

func (d *Wopan) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	_type := 1
	if srcObj.IsDir() {
		_type = 0
	}
	return d.client.RenameFileOrDirectory(d.getSpaceType(), _type, srcObj.GetID(), newName, d.FamilyID, func(req *resty.Request) {
		req.SetContext(ctx)
	})
}

func (d *Wopan) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	dirList := make([]string, 0)
	fileList := make([]string, 0)
	if srcObj.IsDir() {
		dirList = append(dirList, srcObj.GetID())
	} else {
		fileList = append(fileList, srcObj.GetID())
	}
	return d.client.CopyFile(dirList, fileList, dstDir.GetID(),
		d.getSpaceType(), d.getSpaceType(),
		d.FamilyID, d.FamilyID, func(req *resty.Request) {
			req.SetContext(ctx)
		})
}

func (d *Wopan) Remove(ctx context.Context, obj model.Obj) error {
	dirList := make([]string, 0)
	fileList := make([]string, 0)
	if obj.IsDir() {
		dirList = append(dirList, obj.GetID())
	} else {
		fileList = append(fileList, obj.GetID())
	}
	return d.client.DeleteFile(d.getSpaceType(), dirList, fileList, func(req *resty.Request) {
		req.SetContext(ctx)
	})
}

func (d *Wopan) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	_, err := d.client.Upload2C(d.getSpaceType(), wopan.Upload2CFile{
		Name:        stream.GetName(),
		Size:        stream.GetSize(),
		Content:     driver.NewLimitedUploadStream(ctx, stream),
		ContentType: stream.GetMimetype(),
	}, dstDir.GetID(), d.FamilyID, wopan.Upload2COption{
		OnProgress: func(current, total int64) {
			up(100 * float64(current) / float64(total))
		},
		Ctx: ctx,
	})
	return err
}

//func (d *Wopan) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Wopan)(nil)
