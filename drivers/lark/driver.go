package lark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	"golang.org/x/time/rate"
)

type Lark struct {
	model.Storage
	Addition

	client          *lark.Client
	rootFolderToken string
}

func (c *Lark) Config() driver.Config {
	return config
}

func (c *Lark) GetAddition() driver.Additional {
	return &c.Addition
}

func (c *Lark) Init(ctx context.Context) error {
	c.client = lark.NewClient(c.AppId, c.AppSecret, lark.WithTokenCache(newTokenCache()))

	paths := strings.Split(c.RootFolderPath, "/")
	token := ""

	var ok bool
	var file *larkdrive.File
	for _, p := range paths {
		if p == "" {
			token = ""
			continue
		}

		resp, err := c.client.Drive.File.ListByIterator(ctx, larkdrive.NewListFileReqBuilder().FolderToken(token).Build())
		if err != nil {
			return err
		}

		for {
			ok, file, err = resp.Next()
			if !ok {
				return errs.ObjectNotFound
			}

			if err != nil {
				return err
			}

			if *file.Type == "folder" && *file.Name == p {
				token = *file.Token
				break
			}
		}
	}

	c.rootFolderToken = token

	return nil
}

func (c *Lark) Drop(ctx context.Context) error {
	return nil
}

func (c *Lark) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	token, ok := c.getObjToken(ctx, dir.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	if token == emptyFolderToken {
		return nil, nil
	}

	resp, err := c.client.Drive.File.ListByIterator(ctx, larkdrive.NewListFileReqBuilder().FolderToken(token).Build())
	if err != nil {
		return nil, err
	}

	ok = false
	var file *larkdrive.File
	var res []model.Obj

	for {
		ok, file, err = resp.Next()
		if !ok {
			break
		}

		if err != nil {
			return nil, err
		}

		modifiedUnix, _ := strconv.ParseInt(*file.ModifiedTime, 10, 64)
		createdUnix, _ := strconv.ParseInt(*file.CreatedTime, 10, 64)

		f := model.Object{
			ID:       *file.Token,
			Path:     strings.Join([]string{c.RootFolderPath, dir.GetPath(), *file.Name}, "/"),
			Name:     *file.Name,
			Size:     0,
			Modified: time.Unix(modifiedUnix, 0),
			Ctime:    time.Unix(createdUnix, 0),
			IsFolder: *file.Type == "folder",
		}
		res = append(res, &f)
	}

	return res, nil
}

func (c *Lark) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	token, ok := c.getObjToken(ctx, file.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	resp, err := c.client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID:     c.AppId,
		AppSecret: c.AppSecret,
	})

	if err != nil {
		return nil, err
	}

	if !c.ExternalMode {
		accessToken := resp.TenantAccessToken

		url := fmt.Sprintf("https://open.feishu.cn/open-apis/drive/v1/files/%s/download", token)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		req.Header.Set("Range", "bytes=0-1")

		ar, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if ar.StatusCode != http.StatusPartialContent {
			return nil, errors.New("failed to get download link")
		}

		return &model.Link{
			URL: url,
			Header: http.Header{
				"Authorization": []string{fmt.Sprintf("Bearer %s", accessToken)},
			},
		}, nil
	} else {
		url := strings.Join([]string{c.TenantUrlPrefix, "file", token}, "/")

		return &model.Link{
			URL: url,
		}, nil
	}
}

func (c *Lark) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	token, ok := c.getObjToken(ctx, parentDir.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	body, err := larkdrive.NewCreateFolderFilePathReqBodyBuilder().FolderToken(token).Name(dirName).Build()
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Drive.File.CreateFolder(ctx,
		larkdrive.NewCreateFolderFileReqBuilder().Body(body).Build())
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, errors.New(resp.Error())
	}

	return &model.Object{
		ID:       *resp.Data.Token,
		Path:     strings.Join([]string{c.RootFolderPath, parentDir.GetPath(), dirName}, "/"),
		Name:     dirName,
		Size:     0,
		IsFolder: true,
	}, nil
}

func (c *Lark) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	srcToken, ok := c.getObjToken(ctx, srcObj.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	dstDirToken, ok := c.getObjToken(ctx, dstDir.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	req := larkdrive.NewMoveFileReqBuilder().
		Body(larkdrive.NewMoveFileReqBodyBuilder().
			Type("file").
			FolderToken(dstDirToken).
			Build()).FileToken(srcToken).
		Build()

	// 发起请求
	resp, err := c.client.Drive.File.Move(ctx, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, errors.New(resp.Error())
	}

	return nil, nil
}

func (c *Lark) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	// TODO rename obj, optional
	return nil, errs.NotImplement
}

func (c *Lark) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	srcToken, ok := c.getObjToken(ctx, srcObj.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	dstDirToken, ok := c.getObjToken(ctx, dstDir.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	req := larkdrive.NewCopyFileReqBuilder().
		Body(larkdrive.NewCopyFileReqBodyBuilder().
			Name(srcObj.GetName()).
			Type("file").
			FolderToken(dstDirToken).
			Build()).FileToken(srcToken).
		Build()

	// 发起请求
	resp, err := c.client.Drive.File.Copy(ctx, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, errors.New(resp.Error())
	}

	return nil, nil
}

func (c *Lark) Remove(ctx context.Context, obj model.Obj) error {
	token, ok := c.getObjToken(ctx, obj.GetPath())
	if !ok {
		return errs.ObjectNotFound
	}

	req := larkdrive.NewDeleteFileReqBuilder().
		FileToken(token).
		Type("file").
		Build()

	// 发起请求
	resp, err := c.client.Drive.File.Delete(ctx, req)
	if err != nil {
		return err
	}

	if !resp.Success() {
		return errors.New(resp.Error())
	}

	return nil
}

var uploadLimit = rate.NewLimiter(rate.Every(time.Second), 5)

func (c *Lark) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	token, ok := c.getObjToken(ctx, dstDir.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	// prepare
	req := larkdrive.NewUploadPrepareFileReqBuilder().
		FileUploadInfo(larkdrive.NewFileUploadInfoBuilder().
			FileName(stream.GetName()).
			ParentType(`explorer`).
			ParentNode(token).
			Size(int(stream.GetSize())).
			Build()).
		Build()

	// 发起请求
	err := uploadLimit.Wait(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Drive.File.UploadPrepare(ctx, req)
	if err != nil {
		return nil, err
	}

	if !resp.Success() {
		return nil, errors.New(resp.Error())
	}

	uploadId := *resp.Data.UploadId
	blockSize := *resp.Data.BlockSize
	blockCount := *resp.Data.BlockNum

	// upload
	for i := 0; i < blockCount; i++ {
		length := int64(blockSize)
		if i == blockCount-1 {
			length = stream.GetSize() - int64(i*blockSize)
		}

		reader := driver.NewLimitedUploadStream(ctx, io.LimitReader(stream, length))

		req := larkdrive.NewUploadPartFileReqBuilder().
			Body(larkdrive.NewUploadPartFileReqBodyBuilder().
				UploadId(uploadId).
				Seq(i).
				Size(int(length)).
				File(reader).
				Build()).
			Build()

		// 发起请求
		err = uploadLimit.Wait(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Drive.File.UploadPart(ctx, req)

		if err != nil {
			return nil, err
		}

		if !resp.Success() {
			return nil, errors.New(resp.Error())
		}

		up(float64(i) / float64(blockCount))
	}

	//close
	closeReq := larkdrive.NewUploadFinishFileReqBuilder().
		Body(larkdrive.NewUploadFinishFileReqBodyBuilder().
			UploadId(uploadId).
			BlockNum(blockCount).
			Build()).
		Build()

	// 发起请求
	closeResp, err := c.client.Drive.File.UploadFinish(ctx, closeReq)
	if err != nil {
		return nil, err
	}

	if !closeResp.Success() {
		return nil, errors.New(closeResp.Error())
	}

	return &model.Object{
		ID: *closeResp.Data.FileToken,
	}, nil
}

//func (d *Lark) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Lark)(nil)
