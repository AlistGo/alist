package bitqiu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/cookiejar"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	streamPkg "github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
)

const (
	baseURL             = "https://pan.bitqiu.com"
	loginURL            = baseURL + "/loginServer/login"
	userInfoURL         = baseURL + "/user/getInfo"
	listURL             = baseURL + "/apiToken/cfi/fs/resources/pages"
	uploadInitializeURL = baseURL + "/apiToken/cfi/fs/upload/v2/initialize"
	uploadCompleteURL   = baseURL + "/apiToken/cfi/fs/upload/v2/complete"
	downloadURL         = baseURL + "/download/getUrl"
	createDirURL        = baseURL + "/resource/create"
	moveResourceURL     = baseURL + "/resource/remove"
	renameResourceURL   = baseURL + "/resource/rename"
	copyResourceURL     = baseURL + "/apiToken/cfi/fs/async/copy"
	copyManagerURL      = baseURL + "/apiToken/cfi/fs/async/manager"
	deleteResourceURL   = baseURL + "/resource/delete"

	successCode       = "10200"
	uploadSuccessCode = "30010"
	copySubmittedCode = "10300"
	orgChannel        = "default|default|default"
)

const (
	copyPollInterval    = time.Second
	copyPollMaxAttempts = 60
	chunkSize           = int64(1 << 20)
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

type BitQiu struct {
	model.Storage
	Addition

	client *resty.Client
	userID string
}

func (d *BitQiu) Config() driver.Config {
	return config
}

func (d *BitQiu) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *BitQiu) Init(ctx context.Context) error {
	if d.Addition.UserPlatform == "" {
		d.Addition.UserPlatform = uuid.NewString()
		op.MustSaveDriverStorage(d)
	}

	if d.client == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return err
		}
		d.client = base.NewRestyClient()
		d.client.SetBaseURL(baseURL)
		d.client.SetCookieJar(jar)
	}
	d.client.SetHeader("user-agent", d.userAgent())

	return d.login(ctx)
}

func (d *BitQiu) Drop(ctx context.Context) error {
	d.client = nil
	d.userID = ""
	return nil
}

func (d *BitQiu) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	parentID := d.resolveParentID(dir)
	dirPath := ""
	if dir != nil {
		dirPath = dir.GetPath()
	}
	pageSize := d.pageSize()
	orderType := d.orderType()
	desc := d.orderDesc()

	var results []model.Obj
	page := 1
	for {
		form := map[string]string{
			"parentId":    parentID,
			"limit":       strconv.Itoa(pageSize),
			"orderType":   orderType,
			"desc":        desc,
			"model":       "1",
			"userId":      d.userID,
			"currentPage": strconv.Itoa(page),
			"page":        strconv.Itoa(page),
			"org_channel": orgChannel,
		}
		var resp Response[ResourcePage]
		if err := d.postForm(ctx, listURL, form, &resp); err != nil {
			return nil, err
		}
		if resp.Code != successCode {
			if resp.Code == "10401" || resp.Code == "10404" {
				if err := d.login(ctx); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("list failed: %s", resp.Message)
		}

		objs, err := utils.SliceConvert(resp.Data.Data, func(item Resource) (model.Obj, error) {
			return item.toObject(parentID, dirPath)
		})
		if err != nil {
			return nil, err
		}
		results = append(results, objs...)

		if !resp.Data.HasNext || len(resp.Data.Data) == 0 {
			break
		}
		page++
	}

	return results, nil
}

func (d *BitQiu) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.NotFile
	}
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	form := map[string]string{
		"fileIds":     file.GetID(),
		"org_channel": orgChannel,
	}
	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[DownloadData]
		if err := d.postForm(ctx, downloadURL, form, &resp); err != nil {
			return nil, err
		}
		switch resp.Code {
		case successCode:
			if resp.Data.URL == "" {
				return nil, fmt.Errorf("empty download url returned")
			}
			return &model.Link{URL: resp.Data.URL}, nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("get link failed: %s", resp.Message)
		}
	}
	return nil, fmt.Errorf("get link failed: retry limit reached")
}

func (d *BitQiu) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	parentID := d.resolveParentID(parentDir)
	parentPath := ""
	if parentDir != nil {
		parentPath = parentDir.GetPath()
	}
	form := map[string]string{
		"parentId":    parentID,
		"name":        dirName,
		"org_channel": orgChannel,
	}
	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[CreateDirData]
		if err := d.postForm(ctx, createDirURL, form, &resp); err != nil {
			return nil, err
		}
		switch resp.Code {
		case successCode:
			newParentID := parentID
			if resp.Data.ParentID != "" {
				newParentID = resp.Data.ParentID
			}
			name := resp.Data.Name
			if name == "" {
				name = dirName
			}
			resource := Resource{
				ResourceID:   resp.Data.DirID,
				ResourceType: 1,
				Name:         name,
				ParentID:     newParentID,
			}
			obj, err := resource.toObject(newParentID, parentPath)
			if err != nil {
				return nil, err
			}
			if o, ok := obj.(*Object); ok {
				o.ParentID = newParentID
			}
			return obj, nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("create folder failed: %s", resp.Message)
		}
	}
	return nil, fmt.Errorf("create folder failed: retry limit reached")
}

func (d *BitQiu) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	targetParentID := d.resolveParentID(dstDir)
	form := map[string]string{
		"dirIds":      "",
		"fileIds":     "",
		"parentId":    targetParentID,
		"org_channel": orgChannel,
	}
	if srcObj.IsDir() {
		form["dirIds"] = srcObj.GetID()
	} else {
		form["fileIds"] = srcObj.GetID()
	}

	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[any]
		if err := d.postForm(ctx, moveResourceURL, form, &resp); err != nil {
			return nil, err
		}
		switch resp.Code {
		case successCode:
			dstPath := ""
			if dstDir != nil {
				dstPath = dstDir.GetPath()
			}
			if setter, ok := srcObj.(model.SetPath); ok {
				setter.SetPath(path.Join(dstPath, srcObj.GetName()))
			}
			if o, ok := srcObj.(*Object); ok {
				o.ParentID = targetParentID
			}
			return srcObj, nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("move failed: %s", resp.Message)
		}
	}
	return nil, fmt.Errorf("move failed: retry limit reached")
}

func (d *BitQiu) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	form := map[string]string{
		"resourceId":  srcObj.GetID(),
		"name":        newName,
		"type":        "0",
		"org_channel": orgChannel,
	}
	if srcObj.IsDir() {
		form["type"] = "1"
	}

	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[any]
		if err := d.postForm(ctx, renameResourceURL, form, &resp); err != nil {
			return nil, err
		}
		switch resp.Code {
		case successCode:
			return updateObjectName(srcObj, newName), nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("rename failed: %s", resp.Message)
		}
	}
	return nil, fmt.Errorf("rename failed: retry limit reached")
}

func (d *BitQiu) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	targetParentID := d.resolveParentID(dstDir)
	form := map[string]string{
		"dirIds":      "",
		"fileIds":     "",
		"parentId":    targetParentID,
		"org_channel": orgChannel,
	}
	if srcObj.IsDir() {
		form["dirIds"] = srcObj.GetID()
	} else {
		form["fileIds"] = srcObj.GetID()
	}

	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[any]
		if err := d.postForm(ctx, copyResourceURL, form, &resp); err != nil {
			return nil, err
		}
		switch resp.Code {
		case successCode, copySubmittedCode:
			return d.waitForCopiedObject(ctx, srcObj, dstDir)
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("copy failed: %s", resp.Message)
		}
	}

	return nil, fmt.Errorf("copy failed: retry limit reached")
}

func (d *BitQiu) Remove(ctx context.Context, obj model.Obj) error {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return err
		}
	}

	form := map[string]string{
		"dirIds":      "",
		"fileIds":     "",
		"org_channel": orgChannel,
	}
	if obj.IsDir() {
		form["dirIds"] = obj.GetID()
	} else {
		form["fileIds"] = obj.GetID()
	}

	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[any]
		if err := d.postForm(ctx, deleteResourceURL, form, &resp); err != nil {
			return err
		}
		switch resp.Code {
		case successCode:
			return nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("remove failed: %s", resp.Message)
		}
	}
	return fmt.Errorf("remove failed: retry limit reached")
}

func (d *BitQiu) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	up(0)
	tmpFile, md5sum, err := streamPkg.CacheFullInTempFileAndHash(file, utils.MD5)
	if err != nil {
		return nil, err
	}
	defer tmpFile.Close()

	parentID := d.resolveParentID(dstDir)
	parentPath := ""
	if dstDir != nil {
		parentPath = dstDir.GetPath()
	}
	form := map[string]string{
		"parentId":    parentID,
		"name":        file.GetName(),
		"size":        strconv.FormatInt(file.GetSize(), 10),
		"hash":        md5sum,
		"sampleMd5":   md5sum,
		"org_channel": orgChannel,
	}
	var resp Response[json.RawMessage]
	if err = d.postForm(ctx, uploadInitializeURL, form, &resp); err != nil {
		return nil, err
	}
	if resp.Code != uploadSuccessCode {
		switch resp.Code {
		case successCode:
			var initData UploadInitData
			if err := json.Unmarshal(resp.Data, &initData); err != nil {
				return nil, fmt.Errorf("parse upload init response failed: %w", err)
			}
			serverCode, err := d.uploadFileInChunks(ctx, tmpFile, file.GetSize(), md5sum, initData, up)
			if err != nil {
				return nil, err
			}
			obj, err := d.completeChunkUpload(ctx, initData, parentID, parentPath, file.GetName(), file.GetSize(), md5sum, serverCode)
			if err != nil {
				return nil, err
			}
			up(100)
			return obj, nil
		default:
			return nil, fmt.Errorf("upload failed: %s", resp.Message)
		}
	}

	var resource Resource
	if err := json.Unmarshal(resp.Data, &resource); err != nil {
		return nil, fmt.Errorf("parse upload response failed: %w", err)
	}
	obj, err := resource.toObject(parentID, parentPath)
	if err != nil {
		return nil, err
	}
	up(100)
	return obj, nil
}

func (d *BitQiu) uploadFileInChunks(ctx context.Context, tmpFile model.File, size int64, md5sum string, initData UploadInitData, up driver.UpdateProgress) (string, error) {
	if d.client == nil {
		return "", fmt.Errorf("client not initialized")
	}
	if size <= 0 {
		return "", fmt.Errorf("invalid file size")
	}
	buf := make([]byte, chunkSize)
	offset := int64(0)
	var finishedFlag string

	for offset < size {
		chunkLen := chunkSize
		remaining := size - offset
		if remaining < chunkLen {
			chunkLen = remaining
		}

		reader := io.NewSectionReader(tmpFile, offset, chunkLen)
		chunkBuf := buf[:chunkLen]
		if _, err := io.ReadFull(reader, chunkBuf); err != nil {
			return "", fmt.Errorf("read chunk failed: %w", err)
		}

		headers := map[string]string{
			"accept":       "*/*",
			"content-type": "application/octet-stream",
			"appid":        initData.AppID,
			"token":        initData.Token,
			"userid":       strconv.FormatInt(initData.UserID, 10),
			"serialnumber": initData.SerialNumber,
			"hash":         md5sum,
			"len":          strconv.FormatInt(chunkLen, 10),
			"offset":       strconv.FormatInt(offset, 10),
			"user-agent":   d.userAgent(),
		}

		var chunkResp ChunkUploadResponse
		req := d.client.R().
			SetContext(ctx).
			SetHeaders(headers).
			SetBody(chunkBuf).
			SetResult(&chunkResp)

		if _, err := req.Post(initData.UploadURL); err != nil {
			return "", err
		}
		if chunkResp.ErrCode != 0 {
			return "", fmt.Errorf("chunk upload failed with code %d", chunkResp.ErrCode)
		}
		finishedFlag = chunkResp.FinishedFlag
		offset += chunkLen
		up(float64(offset) * 100 / float64(size))
	}

	if finishedFlag == "" {
		return "", fmt.Errorf("upload finished without server code")
	}
	return finishedFlag, nil
}

func (d *BitQiu) completeChunkUpload(ctx context.Context, initData UploadInitData, parentID, parentPath, name string, size int64, md5sum, serverCode string) (model.Obj, error) {
	form := map[string]string{
		"currentPage": "1",
		"limit":       "1",
		"userId":      strconv.FormatInt(initData.UserID, 10),
		"status":      "0",
		"parentId":    parentID,
		"name":        name,
		"fileUid":     initData.FileUID,
		"fileSid":     initData.FileSID,
		"size":        strconv.FormatInt(size, 10),
		"serverCode":  serverCode,
		"snapTime":    "",
		"hash":        md5sum,
		"sampleMd5":   md5sum,
		"org_channel": orgChannel,
	}

	var resp Response[Resource]
	if err := d.postForm(ctx, uploadCompleteURL, form, &resp); err != nil {
		return nil, err
	}
	if resp.Code != successCode {
		return nil, fmt.Errorf("complete upload failed: %s", resp.Message)
	}

	return resp.Data.toObject(parentID, parentPath)
}

func (d *BitQiu) login(ctx context.Context) error {
	if d.client == nil {
		return fmt.Errorf("client not initialized")
	}

	form := map[string]string{
		"passport":    d.Username,
		"password":    utils.GetMD5EncodeStr(d.Password),
		"remember":    "0",
		"captcha":     "",
		"org_channel": orgChannel,
	}
	var resp Response[LoginData]
	if err := d.postForm(ctx, loginURL, form, &resp); err != nil {
		return err
	}
	if resp.Code != successCode {
		return fmt.Errorf("login failed: %s", resp.Message)
	}
	d.userID = strconv.FormatInt(resp.Data.UserID, 10)
	return d.ensureRootFolderID(ctx)
}

func (d *BitQiu) ensureRootFolderID(ctx context.Context) error {
	rootID := d.Addition.GetRootId()
	if rootID != "" && rootID != "0" {
		return nil
	}

	form := map[string]string{
		"org_channel": orgChannel,
	}
	var resp Response[UserInfoData]
	if err := d.postForm(ctx, userInfoURL, form, &resp); err != nil {
		return err
	}
	if resp.Code != successCode {
		return fmt.Errorf("get user info failed: %s", resp.Message)
	}
	if resp.Data.RootDirID == "" {
		return fmt.Errorf("get user info failed: empty root dir id")
	}
	if d.Addition.RootFolderID != resp.Data.RootDirID {
		d.Addition.RootFolderID = resp.Data.RootDirID
		op.MustSaveDriverStorage(d)
	}
	return nil
}

func (d *BitQiu) postForm(ctx context.Context, url string, form map[string]string, result interface{}) error {
	if d.client == nil {
		return fmt.Errorf("client not initialized")
	}
	req := d.client.R().
		SetContext(ctx).
		SetHeaders(d.commonHeaders()).
		SetFormData(form)
	if result != nil {
		req = req.SetResult(result)
	}
	_, err := req.Post(url)
	return err
}

func (d *BitQiu) waitForCopiedObject(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	expectedName := srcObj.GetName()
	expectedIsDir := srcObj.IsDir()
	var lastListErr error

	for attempt := 0; attempt < copyPollMaxAttempts; attempt++ {
		if attempt > 0 {
			if err := waitWithContext(ctx, copyPollInterval); err != nil {
				return nil, err
			}
		}

		if err := d.checkCopyFailure(ctx); err != nil {
			return nil, err
		}

		obj, err := d.findObjectInDir(ctx, dstDir, expectedName, expectedIsDir)
		if err != nil {
			lastListErr = err
			continue
		}
		if obj != nil {
			return obj, nil
		}
	}
	if lastListErr != nil {
		return nil, lastListErr
	}
	return nil, fmt.Errorf("copy task timed out waiting for completion")
}

func (d *BitQiu) checkCopyFailure(ctx context.Context) error {
	form := map[string]string{
		"org_channel": orgChannel,
	}
	for attempt := 0; attempt < 2; attempt++ {
		var resp Response[AsyncManagerData]
		if err := d.postForm(ctx, copyManagerURL, form, &resp); err != nil {
			return err
		}
		switch resp.Code {
		case successCode:
			if len(resp.Data.FailTasks) > 0 {
				return fmt.Errorf("copy failed: %s", resp.Data.FailTasks[0].ErrorMessage())
			}
			return nil
		case "10401", "10404":
			if err := d.login(ctx); err != nil {
				return err
			}
		default:
			return fmt.Errorf("query copy status failed: %s", resp.Message)
		}
	}
	return fmt.Errorf("query copy status failed: retry limit reached")
}

func (d *BitQiu) findObjectInDir(ctx context.Context, dir model.Obj, name string, isDir bool) (model.Obj, error) {
	objs, err := d.List(ctx, dir, model.ListArgs{})
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		if obj.GetName() == name && obj.IsDir() == isDir {
			return obj, nil
		}
	}
	return nil, nil
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *BitQiu) commonHeaders() map[string]string {
	headers := map[string]string{
		"accept":                 "application/json, text/plain, */*",
		"accept-language":        "en-US,en;q=0.9",
		"cache-control":          "no-cache",
		"pragma":                 "no-cache",
		"user-platform":          d.Addition.UserPlatform,
		"x-kl-saas-ajax-request": "Ajax_Request",
		"x-requested-with":       "XMLHttpRequest",
		"referer":                baseURL + "/",
		"origin":                 baseURL,
		"user-agent":             d.userAgent(),
	}
	return headers
}

func (d *BitQiu) userAgent() string {
	if ua := strings.TrimSpace(d.Addition.UserAgent); ua != "" {
		return ua
	}
	return defaultUserAgent
}

func (d *BitQiu) resolveParentID(dir model.Obj) string {
	if dir != nil && dir.GetID() != "" {
		return dir.GetID()
	}
	if root := d.Addition.GetRootId(); root != "" {
		return root
	}
	return config.DefaultRoot
}

func (d *BitQiu) pageSize() int {
	if size, err := strconv.Atoi(d.Addition.PageSize); err == nil && size > 0 {
		return size
	}
	return 24
}

func (d *BitQiu) orderType() string {
	if d.Addition.OrderType != "" {
		return d.Addition.OrderType
	}
	return "updateTime"
}

func (d *BitQiu) orderDesc() string {
	if d.Addition.OrderDesc {
		return "1"
	}
	return "0"
}

var _ driver.Driver = (*BitQiu)(nil)
var _ driver.PutResult = (*BitQiu)(nil)
