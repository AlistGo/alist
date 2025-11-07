package _189

import (
	"context"
	"net/http"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type Cloud189 struct {
	model.Storage
	Addition
	client     *resty.Client
	rsa        Rsa
	sessionKey string
}

func (d *Cloud189) Config() driver.Config {
	return config
}

func (d *Cloud189) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Cloud189) Init(ctx context.Context) error {
	d.client = base.NewRestyClient().
		SetHeader("Referer", "https://cloud.189.cn/")
	return d.newLogin()
}

func (d *Cloud189) Drop(ctx context.Context) error {
	return nil
}

func (d *Cloud189) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return d.getFiles(dir.GetID())
}

func (d *Cloud189) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var resp DownResp
	u := "https://cloud.189.cn/api/portal/getFileInfo.action"
	_, err := d.request(u, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("fileId", file.GetID())
	}, &resp)
	if err != nil {
		return nil, err
	}
	client := resty.NewWithClient(d.client.GetClient()).SetRedirectPolicy(
		resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}))
	res, err := client.R().SetHeader("User-Agent", base.UserAgent).Get("https:" + resp.FileDownloadUrl)
	if err != nil {
		return nil, err
	}
	log.Debugln(res.Status())
	log.Debugln(res.String())
	link := model.Link{}
	log.Debugln("first url:", resp.FileDownloadUrl)
	if res.StatusCode() == 302 {
		link.URL = res.Header().Get("location")
		log.Debugln("second url:", link.URL)
		_, _ = client.R().Get(link.URL)
		if res.StatusCode() == 302 {
			link.URL = res.Header().Get("location")
		}
		log.Debugln("third url:", link.URL)
	} else {
		link.URL = resp.FileDownloadUrl
	}
	link.URL = strings.Replace(link.URL, "http://", "https://", 1)
	return &link, nil
}

func (d *Cloud189) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	safeName := d.sanitizeName(dirName)
	form := map[string]string{
		"parentFolderId": parentDir.GetID(),
		"folderName":     safeName,
	}
	_, err := d.request("https://cloud.189.cn/api/open/file/createFolder.action", http.MethodPost, func(req *resty.Request) {
		req.SetFormData(form)
	}, nil)
	return err
}

func (d *Cloud189) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	isFolder := 0
	if srcObj.IsDir() {
		isFolder = 1
	}
	taskInfos := []base.Json{
		{
			"fileId":   srcObj.GetID(),
			"fileName": srcObj.GetName(),
			"isFolder": isFolder,
		},
	}
	taskInfosBytes, err := utils.Json.Marshal(taskInfos)
	if err != nil {
		return err
	}
	form := map[string]string{
		"type":           "MOVE",
		"targetFolderId": dstDir.GetID(),
		"taskInfos":      string(taskInfosBytes),
	}
	_, err = d.request("https://cloud.189.cn/api/open/batch/createBatchTask.action", http.MethodPost, func(req *resty.Request) {
		req.SetFormData(form)
	}, nil)
	return err
}

func (d *Cloud189) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	url := "https://cloud.189.cn/api/open/file/renameFile.action"
	idKey := "fileId"
	nameKey := "destFileName"
	if srcObj.IsDir() {
		url = "https://cloud.189.cn/api/open/file/renameFolder.action"
		idKey = "folderId"
		nameKey = "destFolderName"
	}
	safeName := d.sanitizeName(newName)
	form := map[string]string{
		idKey:   srcObj.GetID(),
		nameKey: safeName,
	}
	_, err := d.request(url, http.MethodPost, func(req *resty.Request) {
		req.SetFormData(form)
	}, nil)
	return err
}

func (d *Cloud189) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	isFolder := 0
	if srcObj.IsDir() {
		isFolder = 1
	}
	taskInfos := []base.Json{
		{
			"fileId":   srcObj.GetID(),
			"fileName": srcObj.GetName(),
			"isFolder": isFolder,
		},
	}
	taskInfosBytes, err := utils.Json.Marshal(taskInfos)
	if err != nil {
		return err
	}
	form := map[string]string{
		"type":           "COPY",
		"targetFolderId": dstDir.GetID(),
		"taskInfos":      string(taskInfosBytes),
	}
	_, err = d.request("https://cloud.189.cn/api/open/batch/createBatchTask.action", http.MethodPost, func(req *resty.Request) {
		req.SetFormData(form)
	}, nil)
	return err
}

func (d *Cloud189) Remove(ctx context.Context, obj model.Obj) error {
	isFolder := 0
	if obj.IsDir() {
		isFolder = 1
	}
	taskInfos := []base.Json{
		{
			"fileId":   obj.GetID(),
			"fileName": obj.GetName(),
			"isFolder": isFolder,
		},
	}
	taskInfosBytes, err := utils.Json.Marshal(taskInfos)
	if err != nil {
		return err
	}
	form := map[string]string{
		"type":           "DELETE",
		"targetFolderId": "",
		"taskInfos":      string(taskInfosBytes),
	}
	_, err = d.request("https://cloud.189.cn/api/open/batch/createBatchTask.action", http.MethodPost, func(req *resty.Request) {
		req.SetFormData(form)
	}, nil)
	return err
}

func (d *Cloud189) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	return d.newUpload(ctx, dstDir, stream, up)
}

var _ driver.Driver = (*Cloud189)(nil)
