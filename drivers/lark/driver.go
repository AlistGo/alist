package lark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	larkext "github.com/larksuite/oapi-sdk-go/v3/service/ext"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

type Lark struct {
	model.Storage
	Addition

	client          *lark.Client
	rootFolderToken string
	tokenMu         sync.Mutex
}

const larkListPageSize = 200
const larkTokenRefreshSkew = 5 * time.Minute

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

	for _, p := range paths {
		if p == "" {
			token = ""
			continue
		}

		files, err := c.listFiles(ctx, token)
		if err != nil {
			return err
		}

		found := false
		for _, file := range files {
			if *file.Type == "folder" && *file.Name == p {
				token = *file.Token
				found = true
				break
			}
		}
		if !found {
			return errs.ObjectNotFound
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

	files, err := c.listFiles(ctx, token)
	if err != nil {
		return nil, err
	}

	var res []model.Obj

	for _, file := range files {
		res = append(res, larkFileToObj(c.RootFolderPath, dir.GetPath(), file))
	}

	return res, nil
}

func larkFileToObj(rootFolderPath, dirPath string, file *larkdrive.File) model.Obj {
	name := larkString(file.Name)
	fileType := larkString(file.Type)
	modifiedUnix, _ := strconv.ParseInt(larkString(file.ModifiedTime), 10, 64)
	createdUnix, _ := strconv.ParseInt(larkString(file.CreatedTime), 10, 64)
	obj := model.Object{
		ID:       larkString(file.Token),
		Path:     strings.Join([]string{rootFolderPath, dirPath, name}, "/"),
		Name:     larkDisplayName(name, fileType),
		Size:     0,
		Modified: time.Unix(modifiedUnix, 0),
		Ctime:    time.Unix(createdUnix, 0),
		IsFolder: fileType == "folder",
	}
	if file.Url == nil || *file.Url == "" || obj.IsFolder || !isLarkNativeDocType(fileType) {
		return &obj
	}
	return &model.ObjectURL{
		Object: obj,
		Url:    model.Url{Url: *file.Url},
	}
}

func larkDisplayName(name, fileType string) string {
	if isLarkCloudDocName(name) {
		return name
	}
	switch fileType {
	case "doc":
		return name + ".lark-doc"
	case "docx":
		return name + ".lark-docx"
	case "sheet":
		return name + ".lark-sheet"
	case "bitable":
		return name + ".lark-bitable"
	case "mindnote":
		return name + ".lark-mindnote"
	case "slides":
		return name + ".lark-slides"
	default:
		return name
	}
}

func isLarkNativeDocType(fileType string) bool {
	switch fileType {
	case "doc", "docx", "sheet", "bitable", "mindnote", "slides":
		return true
	default:
		return false
	}
}

func larkString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c *Lark) requestOpts(ctx context.Context) ([]larkcore.RequestOptionFunc, error) {
	userAccessToken, err := c.ensureUserAccessToken(ctx, false)
	if err != nil {
		return nil, err
	}
	if userAccessToken == "" {
		return nil, nil
	}
	return []larkcore.RequestOptionFunc{larkcore.WithUserAccessToken(userAccessToken)}, nil
}

func (c *Lark) ensureUserAccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	if strings.TrimSpace(c.RefreshToken) == "" {
		return strings.TrimSpace(c.UserAccessToken), nil
	}
	if token := strings.TrimSpace(c.UserAccessToken); !forceRefresh && token != "" && !c.userAccessTokenExpired() {
		return token, nil
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	if token := strings.TrimSpace(c.UserAccessToken); !forceRefresh && token != "" && !c.userAccessTokenExpired() {
		return token, nil
	}
	if c.RefreshTokenExpiresAt > 0 && time.Now().After(time.Unix(c.RefreshTokenExpiresAt, 0)) {
		return "", errors.New("lark refresh token expired")
	}

	resp, err := c.client.Ext.Authen.RefreshAuthenAccessToken(ctx,
		larkext.NewRefreshAuthenAccessTokenReqBuilder().
			Body(larkext.NewRefreshAuthenAccessTokenReqBodyBuilder().
				GrantType(larkext.GrantTypeRefreshCode).
				RefreshToken(strings.TrimSpace(c.RefreshToken)).
				Build()).
			Build())
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", errors.New(resp.Error())
	}
	if resp.Data == nil || resp.Data.AccessToken == "" {
		return "", errors.New("lark refresh token response missing access token")
	}

	now := time.Now()
	c.UserAccessToken = resp.Data.AccessToken
	c.UserAccessTokenExpiresAt = now.Add(time.Duration(resp.Data.ExpiresIn) * time.Second).Unix()
	if resp.Data.RefreshToken != "" {
		c.RefreshToken = resp.Data.RefreshToken
	}
	if resp.Data.RefreshExpiresIn > 0 {
		c.RefreshTokenExpiresAt = now.Add(time.Duration(resp.Data.RefreshExpiresIn) * time.Second).Unix()
	}
	op.MustSaveDriverStorage(c)

	return c.UserAccessToken, nil
}

func (c *Lark) forceRefreshUserAccessToken(ctx context.Context) error {
	if strings.TrimSpace(c.RefreshToken) == "" {
		return nil
	}
	_, err := c.ensureUserAccessToken(ctx, true)
	return err
}

func (c *Lark) userAccessTokenExpired() bool {
	if c.UserAccessTokenExpiresAt <= 0 {
		return true
	}
	return time.Now().Add(larkTokenRefreshSkew).After(time.Unix(c.UserAccessTokenExpiresAt, 0))
}

func (c *Lark) listFiles(ctx context.Context, folderToken string) ([]*larkdrive.File, error) {
	var files []*larkdrive.File
	pageToken := ""

	for {
		builder := larkdrive.NewListFileReqBuilder().
			FolderToken(folderToken).
			OrderBy("EditedTime").
			Direction("DESC")
		if folderToken != "" {
			builder.PageSize(larkListPageSize)
			if pageToken != "" {
				builder.PageToken(pageToken)
			}
		}

		resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.ListFileResp, error) {
			return c.client.Drive.V1.File.List(ctx, builder.Build(), opts...)
		})
		if err != nil {
			return nil, err
		}
		if !resp.Success() {
			return nil, errors.New(resp.Error())
		}
		if resp.Data == nil {
			return files, nil
		}

		files = append(files, resp.Data.Files...)
		if folderToken == "" || resp.Data.HasMore == nil || !*resp.Data.HasMore ||
			resp.Data.NextPageToken == nil || *resp.Data.NextPageToken == "" {
			break
		}
		pageToken = *resp.Data.NextPageToken
	}

	return files, nil
}

type larkResp interface {
	Success() bool
	Error() string
}

func doDrive[T larkResp](ctx context.Context, c *Lark, call func(...larkcore.RequestOptionFunc) (T, error)) (T, error) {
	opts, err := c.requestOpts(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	resp, err := call(opts...)
	if err != nil {
		var zero T
		return zero, err
	}
	if !isLarkAuthFailed(resp) || strings.TrimSpace(c.RefreshToken) == "" {
		return resp, nil
	}

	log.WithField("mount_path", c.MountPath).Warn("lark user access token auth failed, refreshing and retrying once")
	if err = c.forceRefreshUserAccessToken(ctx); err != nil {
		return resp, nil
	}
	opts, err = c.requestOpts(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	return call(opts...)
}

func isLarkAuthFailed(resp larkResp) bool {
	if resp == nil || resp.Success() {
		return false
	}
	switch v := any(resp).(type) {
	case *larkdrive.ListFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.CreateFolderFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.MoveFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.CopyFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.DeleteFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.UploadPrepareFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.UploadPartFileResp:
		return isLarkAuthFailedCode(v.Code)
	case *larkdrive.UploadFinishFileResp:
		return isLarkAuthFailedCode(v.Code)
	default:
		return strings.Contains(resp.Error(), "1061005") ||
			strings.Contains(strings.ToLower(resp.Error()), "auth")
	}
}

func isLarkAuthFailedCode(code int) bool {
	return code == 1061005 || code == 99991663 || code == 99991664 || code == 99991668
}

func isHTTPAuthFailed(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

func (c *Lark) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	token, ok := c.getObjToken(ctx, file.GetPath())
	if !ok {
		return nil, errs.ObjectNotFound
	}

	if isLarkCloudDocName(file.GetName()) {
		return &model.Link{
			URL: c.filePreviewURL(token),
		}, nil
	}

	if !c.WebProxy || c.ExternalMode {
		return &model.Link{
			URL: c.filePreviewURL(token),
		}, nil
	}

	if c.WebProxy {
		accessToken, err := c.downloadAccessToken(ctx, false)
		if err != nil {
			return nil, err
		}

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
		_ = ar.Body.Close()

		if isHTTPAuthFailed(ar.StatusCode) && strings.TrimSpace(c.RefreshToken) != "" {
			accessToken, err = c.downloadAccessToken(ctx, true)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
			ar, err = http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			_ = ar.Body.Close()
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
	}

	return nil, errors.New("lark download requires web proxy")
}

func (c *Lark) filePreviewURL(token string) string {
	prefix := strings.TrimRight(strings.TrimSpace(c.TenantUrlPrefix), "/")
	if prefix == "" {
		prefix = "https://www.feishu.cn"
	}
	return prefix + "/file/" + token
}

func (c *Lark) downloadAccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	var accessToken string
	var err error
	if strings.TrimSpace(c.RefreshToken) != "" || strings.TrimSpace(c.UserAccessToken) != "" {
		accessToken, err = c.ensureUserAccessToken(ctx, forceRefresh)
		if err != nil {
			return "", err
		}
	}
	if accessToken != "" {
		return accessToken, nil
	}

	resp, err := c.client.GetTenantAccessTokenBySelfBuiltApp(ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
		AppID:     c.AppId,
		AppSecret: c.AppSecret,
	})
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", errors.New(resp.Error())
	}
	if resp.TenantAccessToken == "" {
		return "", errors.New("lark tenant access token is empty")
	}
	return resp.TenantAccessToken, nil
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

	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.CreateFolderFileResp, error) {
		return c.client.Drive.File.CreateFolder(ctx,
			larkdrive.NewCreateFolderFileReqBuilder().Body(body).Build(), opts...)
	})
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
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.MoveFileResp, error) {
		return c.client.Drive.File.Move(ctx, req, opts...)
	})
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
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.CopyFileResp, error) {
		return c.client.Drive.File.Copy(ctx, req, opts...)
	})
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
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.DeleteFileResp, error) {
		return c.client.Drive.File.Delete(ctx, req, opts...)
	})
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
	resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.UploadPrepareFileResp, error) {
		return c.client.Drive.File.UploadPrepare(ctx, req, opts...)
	})
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
		resp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.UploadPartFileResp, error) {
			return c.client.Drive.File.UploadPart(ctx, req, opts...)
		})

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
	closeResp, err := doDrive(ctx, c, func(opts ...larkcore.RequestOptionFunc) (*larkdrive.UploadFinishFileResp, error) {
		return c.client.Drive.File.UploadFinish(ctx, closeReq, opts...)
	})
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
