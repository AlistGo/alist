package onedrive_app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdpath "path"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
)

var onedriveHostMap = map[string]Host{
	"global": {
		Oauth: "https://login.microsoftonline.com",
		Api:   "https://graph.microsoft.com",
	},
	"cn": {
		Oauth: "https://login.chinacloudapi.cn",
		Api:   "https://microsoftgraph.chinacloudapi.cn",
	},
	"us": {
		Oauth: "https://login.microsoftonline.us",
		Api:   "https://graph.microsoft.us",
	},
	"de": {
		Oauth: "https://login.microsoftonline.de",
		Api:   "https://graph.microsoft.de",
	},
}

func (d *OnedriveAPP) GetMetaUrl(auth bool, path string) string {
	host, _ := onedriveHostMap[d.Region]
	path = utils.EncodePath(path, true)
	if auth {
		return host.Oauth
	}
	if path == "/" || path == "\\" {
		return fmt.Sprintf("%s/v1.0/users/%s/drive/root", host.Api, d.Email)
	}
	return fmt.Sprintf("%s/v1.0/users/%s/drive/root:%s:", host.Api, d.Email, path)
}

func (d *OnedriveAPP) accessToken() error {
	var err error
	for i := 0; i < 3; i++ {
		err = d._accessToken()
		if err == nil {
			break
		}
	}
	return err
}

func (d *OnedriveAPP) _accessToken() error {
	url := d.GetMetaUrl(true, "") + "/" + d.TenantID + "/oauth2/token"
	var resp base.TokenResp
	var e TokenErr
	_, err := base.RestyClient.R().SetResult(&resp).SetError(&e).SetFormData(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
		"resource":      onedriveHostMap[d.Region].Api + "/",
		"scope":         onedriveHostMap[d.Region].Api + "/.default",
	}).Post(url)
	if err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("%s", e.ErrorDescription)
	}
	if resp.AccessToken == "" {
		return errs.EmptyToken
	}
	d.AccessToken = resp.AccessToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *OnedriveAPP) Request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()
	req.SetHeader("Authorization", "Bearer "+d.AccessToken)
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e RespErr
	req.SetError(&e)
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	if e.Error.Code != "" {
		if e.Error.Code == "InvalidAuthenticationToken" {
			err = d.accessToken()
			if err != nil {
				return nil, err
			}
			return d.Request(url, method, callback, resp)
		}
		return nil, errors.New(e.Error.Message)
	}
	return res.Body(), nil
}

func (d *OnedriveAPP) getFiles(path string) ([]File, error) {
	var res []File
	nextLink := d.GetMetaUrl(false, path) + "/children?$top=1000&$expand=thumbnails($select=medium)&$select=id,name,size,lastModifiedDateTime,content.downloadUrl,file,parentReference"
	for nextLink != "" {
		var files Files
		_, err := d.Request(nextLink, http.MethodGet, nil, &files)
		if err != nil {
			return nil, err
		}
		res = append(res, files.Value...)
		nextLink = files.NextLink
	}
	return res, nil
}

func (d *OnedriveAPP) GetFile(path string) (*File, error) {
	var file File
	u := d.GetMetaUrl(false, path)
	_, err := d.Request(u, http.MethodGet, nil, &file)
	return &file, err
}

func (d *OnedriveAPP) upSmall(ctx context.Context, dstDir model.Obj, stream model.FileStreamer) error {
	url := d.GetMetaUrl(false, stdpath.Join(dstDir.GetPath(), stream.GetName())) + "/content"
	_, err := d.Request(url, http.MethodPut, func(req *resty.Request) {
		req.SetBody(driver.NewLimitedUploadStream(ctx, stream)).SetContext(ctx)
	}, nil)
	return err
}

func (d *OnedriveAPP) upBig(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	url := d.GetMetaUrl(false, stdpath.Join(dstDir.GetPath(), stream.GetName())) + "/createUploadSession"
	res, err := d.Request(url, http.MethodPost, nil, nil)
	if err != nil {
		return err
	}
	uploadUrl := jsoniter.Get(res, "uploadUrl").ToString()
	var finish int64 = 0
	DEFAULT := d.ChunkSize * 1024 * 1024
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		log.Debugf("upload: %d", finish)
		var byteSize int64 = DEFAULT
		left := stream.GetSize() - finish
		if left < DEFAULT {
			byteSize = left
		}
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("PUT", uploadUrl, driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		// req.Header.Set("Content-Length", strconv.Itoa(int(byteSize)))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", finish, finish+byteSize-1, stream.GetSize()))
		finish += byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		// https://learn.microsoft.com/zh-cn/onedrive/developer/rest-api/api/driveitem_createuploadsession
		if res.StatusCode != 201 && res.StatusCode != 202 && res.StatusCode != 200 {
			data, _ := io.ReadAll(res.Body)
			res.Body.Close()
			return errors.New(string(data))
		}
		res.Body.Close()
		up(float64(finish) * 100 / float64(stream.GetSize()))
	}
	return nil
}
