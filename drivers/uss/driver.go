package uss

import (
	"context"
	"fmt"
	"github.com/alist-org/alist/v3/internal/stream"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/upyun/go-sdk/v3/upyun"
)

type USS struct {
	model.Storage
	Addition
	client *upyun.UpYun
}

func (d *USS) Config() driver.Config {
	return config
}

func (d *USS) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *USS) Init(ctx context.Context) error {
	d.client = upyun.NewUpYun(&upyun.UpYunConfig{
		Bucket:   d.Bucket,
		Operator: d.OperatorName,
		Password: d.OperatorPassword,
	})
	return nil
}

func (d *USS) Drop(ctx context.Context) error {
	return nil
}

func (d *USS) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	prefix := getKey(dir.GetPath(), true)
	objsChan := make(chan *upyun.FileInfo, 10)
	var err error
	go func() {
		err = d.client.List(&upyun.GetObjectsConfig{
			Path:           prefix,
			ObjectsChan:    objsChan,
			MaxListObjects: 0,
			MaxListLevel:   1,
		})
	}()
	if err != nil {
		return nil, err
	}
	res := make([]model.Obj, 0)
	for obj := range objsChan {
		t := obj.Time
		f := model.Object{
			Name:     obj.Name,
			Size:     obj.Size,
			Modified: t,
			IsFolder: obj.IsDir,
		}
		res = append(res, &f)
	}
	return res, err
}

func (d *USS) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	key := getKey(file.GetPath(), false)
	host := d.Endpoint
	if !strings.Contains(host, "://") { //判断是否包含协议头，否则https
		host = "https://" + host
	}
	u := fmt.Sprintf("%s/%s", host, key)
	downExp := time.Hour * time.Duration(d.SignURLExpire)
	expireAt := time.Now().Add(downExp).Unix()
	upd := url.QueryEscape(path.Base(file.GetPath()))
	tokenOrPassword := d.AntiTheftChainToken
	if tokenOrPassword == "" {
		tokenOrPassword = d.OperatorPassword
	}
	signStr := strings.Join([]string{tokenOrPassword, fmt.Sprint(expireAt), fmt.Sprintf("/%s", key)}, "&")
	upt := utils.GetMD5EncodeStr(signStr)[12:20] + fmt.Sprint(expireAt)
	link := fmt.Sprintf("%s?_upd=%s&_upt=%s", u, upd, upt)
	return &model.Link{URL: link}, nil
}

func (d *USS) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.client.Mkdir(getKey(path.Join(parentDir.GetPath(), dirName), true))
}

func (d *USS) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.client.Move(&upyun.MoveObjectConfig{
		SrcPath:  getKey(srcObj.GetPath(), srcObj.IsDir()),
		DestPath: getKey(path.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.IsDir()),
	})
}

func (d *USS) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return d.client.Move(&upyun.MoveObjectConfig{
		SrcPath:  getKey(srcObj.GetPath(), srcObj.IsDir()),
		DestPath: getKey(path.Join(path.Dir(srcObj.GetPath()), newName), srcObj.IsDir()),
	})
}

func (d *USS) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.client.Copy(&upyun.CopyObjectConfig{
		SrcPath:  getKey(srcObj.GetPath(), srcObj.IsDir()),
		DestPath: getKey(path.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.IsDir()),
	})
}

func (d *USS) Remove(ctx context.Context, obj model.Obj) error {
	return d.client.Delete(&upyun.DeleteObjectConfig{
		Path:  getKey(obj.GetPath(), obj.IsDir()),
		Async: false,
	})
}

func (d *USS) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	return d.client.Put(&upyun.PutObjectConfig{
		Path: getKey(path.Join(dstDir.GetPath(), s.GetName()), false),
		Reader: driver.NewLimitedUploadStream(ctx, &stream.ReaderUpdatingProgress{
			Reader:         s,
			UpdateProgress: up,
		}),
	})
}

var _ driver.Driver = (*USS)(nil)
