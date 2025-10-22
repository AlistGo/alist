package bitqiu

import (
	"context"
	"fmt"
	"net/http/cookiejar"
	"path"
	"strconv"

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
	listURL             = baseURL + "/apiToken/cfi/fs/resources/pages"
	uploadInitializeURL = baseURL + "/apiToken/cfi/fs/upload/v2/initialize"
	downloadURL         = baseURL + "/download/getUrl"
	createDirURL        = baseURL + "/resource/create"
	moveResourceURL     = baseURL + "/resource/remove"

	successCode       = "10200"
	uploadSuccessCode = "30010"
	orgChannel        = "default|default|default"
)

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
	return nil, errs.NotImplement
}

func (d *BitQiu) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *BitQiu) Remove(ctx context.Context, obj model.Obj) error {
	return errs.NotImplement
}

func (d *BitQiu) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if d.userID == "" {
		if err := d.login(ctx); err != nil {
			return nil, err
		}
	}

	up(0)
	_, md5sum, err := streamPkg.CacheFullInTempFileAndHash(file, utils.MD5)
	if err != nil {
		return nil, err
	}

	parentID := d.resolveParentID(dstDir)
	form := map[string]string{
		"parentId":    parentID,
		"name":        file.GetName(),
		"size":        strconv.FormatInt(file.GetSize(), 10),
		"hash":        md5sum,
		"sampleMd5":   md5sum,
		"org_channel": orgChannel,
	}
	var resp Response[Resource]
	if err = d.postForm(ctx, uploadInitializeURL, form, &resp); err != nil {
		return nil, err
	}
	if resp.Code != uploadSuccessCode {
		if resp.Code == successCode {
			return nil, fmt.Errorf("upload requires additional steps not implemented: %s", resp.Message)
		}
		return nil, fmt.Errorf("upload failed: %s", resp.Message)
	}

	obj, err := resp.Data.toObject(parentID, dstDir.GetPath())
	if err != nil {
		return nil, err
	}
	up(100)
	return obj, nil
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
	}
	return headers
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
