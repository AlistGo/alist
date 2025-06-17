package onedrive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdpath "path"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
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

func (d *Onedrive) GetMetaUrl(auth bool, path string) string {
	host, _ := onedriveHostMap[d.Region]
	path = utils.EncodePath(path, true)
	if auth {
		return host.Oauth
	}
	if d.IsSharepoint {
		if path == "/" || path == "\\" {
			return fmt.Sprintf("%s/v1.0/sites/%s/drive/root", host.Api, d.SiteId)
		} else {
			return fmt.Sprintf("%s/v1.0/sites/%s/drive/root:%s:", host.Api, d.SiteId, path)
		}
	} else {
		if path == "/" || path == "\\" {
			return fmt.Sprintf("%s/v1.0/me/drive/root", host.Api)
		} else {
			return fmt.Sprintf("%s/v1.0/me/drive/root:%s:", host.Api, path)
		}
	}
}

func (d *Onedrive) refreshToken() error {
	var err error
	for i := 0; i < 3; i++ {
		err = d._refreshToken()
		if err == nil {
			break
		}
	}
	return err
}

func (d *Onedrive) _refreshToken() error {
	url := d.GetMetaUrl(true, "") + "/common/oauth2/v2.0/token"
	var resp base.TokenResp
	var e TokenErr
	_, err := base.RestyClient.R().SetResult(&resp).SetError(&e).SetFormData(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
		"redirect_uri":  d.RedirectUri,
		"refresh_token": d.RefreshToken,
	}).Post(url)
	if err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("%s", e.ErrorDescription)
	}
	if resp.RefreshToken == "" {
		return errs.EmptyToken
	}
	d.RefreshToken, d.AccessToken = resp.RefreshToken, resp.AccessToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *Onedrive) Request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
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
			err = d.refreshToken()
			if err != nil {
				return nil, err
			}
			return d.Request(url, method, callback, resp)
		}
		return nil, errors.New(e.Error.Message)
	}
	return res.Body(), nil
}

func (d *Onedrive) getFiles(path string) ([]File, error) {
	var res []File
	nextLink := d.GetMetaUrl(false, path) + "/children?$top=1000&$expand=thumbnails($select=medium)&$select=id,name,size,fileSystemInfo,content.downloadUrl,file,parentReference"
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

func (d *Onedrive) GetFile(path string) (*File, error) {
	var file File
	u := d.GetMetaUrl(false, path)
	_, err := d.Request(u, http.MethodGet, nil, &file)
	return &file, err
}

func (d *Onedrive) upSmall(ctx context.Context, dstDir model.Obj, stream model.FileStreamer) error {
	filepath := stdpath.Join(dstDir.GetPath(), stream.GetName())
	// 1. upload new file
	// ApiDoc: https://learn.microsoft.com/en-us/onedrive/developer/rest-api/api/driveitem_put_content?view=odsp-graph-online
	url := d.GetMetaUrl(false, filepath) + "/content"
	_, err := d.Request(url, http.MethodPut, func(req *resty.Request) {
		req.SetBody(driver.NewLimitedUploadStream(ctx, stream)).SetContext(ctx)
	}, nil)
	if err != nil {
		return fmt.Errorf("onedrive: Failed to upload new file(path=%v): %w", filepath, err)
	}

	// 2. update metadata
	err = d.updateMetadata(ctx, stream, filepath)
	if err != nil {
		return fmt.Errorf("onedrive: Failed to update file(path=%v) metadata: %w", filepath, err)
	}
	return nil
}

func (d *Onedrive) updateMetadata(ctx context.Context, stream model.FileStreamer, filepath string) error {
	url := d.GetMetaUrl(false, filepath)
	metadata := toAPIMetadata(stream)
	// ApiDoc: https://learn.microsoft.com/en-us/onedrive/developer/rest-api/api/driveitem_update?view=odsp-graph-online
	_, err := d.Request(url, http.MethodPatch, func(req *resty.Request) {
		req.SetBody(metadata).SetContext(ctx)
	}, nil)
	return err
}

func toAPIMetadata(stream model.FileStreamer) Metadata {
	metadata := Metadata{
		FileSystemInfo: &FileSystemInfoFacet{},
	}
	if !stream.ModTime().IsZero() {
		metadata.FileSystemInfo.LastModifiedDateTime = stream.ModTime()
	}
	if !stream.CreateTime().IsZero() {
		metadata.FileSystemInfo.CreatedDateTime = stream.CreateTime()
	}
	if stream.CreateTime().IsZero() && !stream.ModTime().IsZero() {
		metadata.FileSystemInfo.CreatedDateTime = stream.CreateTime()
	}
	return metadata
}

func (d *Onedrive) upBig(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	url := d.GetMetaUrl(false, stdpath.Join(dstDir.GetPath(), stream.GetName())) + "/createUploadSession"
	metadata := map[string]interface{}{"item": toAPIMetadata(stream)}
	res, err := d.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(metadata).SetContext(ctx)
	}, nil)
	if err != nil {
		return err
	}
	uploadUrl := jsoniter.Get(res, "uploadUrl").ToString()
	var finish int64 = 0
	DEFAULT := d.ChunkSize * 1024 * 1024
	retryCount := 0
	maxRetries := 3
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		left := stream.GetSize() - finish
		byteSize := min(left, DEFAULT)
		utils.Log.Debugf("[Onedrive] upload range: %d-%d/%d", finish, finish+byteSize-1, stream.GetSize())
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		utils.Log.Debug(err, n)
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
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		// https://learn.microsoft.com/zh-cn/onedrive/developer/rest-api/api/driveitem_createuploadsession
		switch {
		case res.StatusCode >= 500 && res.StatusCode <= 504:
			retryCount++
			if retryCount > maxRetries {
				res.Body.Close()
				return fmt.Errorf("upload failed after %d retries due to server errors, error %d", maxRetries, res.StatusCode)
			}
			backoff := time.Duration(1<<retryCount) * time.Second
			utils.Log.Warnf("[Onedrive] server errors %d while uploading, retrying after %v...", res.StatusCode, backoff)
			time.Sleep(backoff)
		case res.StatusCode != 201 && res.StatusCode != 202 && res.StatusCode != 200:
			data, _ := io.ReadAll(res.Body)
			res.Body.Close()
			return errors.New(string(data))
		default:
			res.Body.Close()
			retryCount = 0
			finish += byteSize
			up(float64(finish) * 100 / float64(stream.GetSize()))
		}
	}
	return nil
}
