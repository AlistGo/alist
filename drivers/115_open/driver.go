package _115_open

import (
	"context"
	"fmt"
	"net/http"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	sdk "github.com/xhofe/115-sdk-go"
)

type Open115 struct {
	model.Storage
	Addition
	client *sdk.Client
}

func (d *Open115) Config() driver.Config {
	return config
}

func (d *Open115) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Open115) Init(ctx context.Context) error {
	d.client = sdk.New(sdk.WithRefreshToken(d.Addition.RefreshToken)).SetOnRefreshToken(func(s1, s2 string) {
		d.Addition.AccessToken = s1
		d.Addition.RefreshToken = s2
		op.MustSaveDriverStorage(d)
	})
	_, err := d.client.UserInfo(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (d *Open115) Drop(ctx context.Context) error {
	return nil
}

func (d *Open115) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var res []model.Obj
	pageSize := int64(200)
	offset := int64(0)
	for {
		resp, err := d.client.GetFiles(ctx, &sdk.GetFilesReq{
			CID:    dir.GetID(),
			Limit:  pageSize,
			Offset: offset,
			ASC:    d.Addition.OrderDirection == "asc",
			O:      d.Addition.OrderBy,
			// Cur:     1,
			ShowDir: true,
		})
		if err != nil {
			return nil, err
		}
		res = append(res, utils.MustSliceConvert(resp.Data, func(src sdk.GetFilesResp_File) model.Obj {
			obj := Obj(src)
			return &obj
		})...)
		if len(res) >= int(resp.Count) {
			break
		}
		offset += pageSize
	}
	return res, nil
}

func (d *Open115) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var ua string
	if args.Header != nil {
		ua = args.Header.Get("User-Agent")
	}
	if ua == "" {
		ua = base.UserAgent
	}
	obj, ok := file.(*Obj)
	if !ok {
		return nil, fmt.Errorf("can't convert obj")
	}
	pc := obj.Pc
	resp, err := d.client.DownURL(ctx, pc, ua)
	if err != nil {
		return nil, err
	}
	u, ok := resp[obj.GetID()]
	if !ok {
		return nil, fmt.Errorf("can't get link")
	}
	return &model.Link{
		URL: u.URL.URL,
		Header: http.Header{
			"User-Agent": []string{ua},
		},
	}, nil
}

func (d *Open115) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	// TODO create folder, optional
	return nil, errs.NotImplement
}

func (d *Open115) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO move obj, optional
	return nil, errs.NotImplement
}

func (d *Open115) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	// TODO rename obj, optional
	return nil, errs.NotImplement
}

func (d *Open115) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO copy obj, optional
	return nil, errs.NotImplement
}

func (d *Open115) Remove(ctx context.Context, obj model.Obj) error {
	// TODO remove obj, optional
	return errs.NotImplement
}

func (d *Open115) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// TODO upload file, optional
	return nil, errs.NotImplement
}

func (d *Open115) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Open115) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Open115) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Open115) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Open115)(nil)
