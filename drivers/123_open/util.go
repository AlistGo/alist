package _123_open

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

var ( //不同情况下获取的AccessTokenQPS限制不同 如下模块化易于拓展
	Api = "https://open-api.123pan.com"

	AccessToken    = InitApiInfo(Api+"/api/v1/access_token", 1)
	RefreshToken   = InitApiInfo(Api+"/api/v1/oauth2/access_token", 1)
	UserInfo       = InitApiInfo(Api+"/api/v1/user/info", 1)
	FileList       = InitApiInfo(Api+"/api/v2/file/list", 4)
	DownloadInfo   = InitApiInfo(Api+"/api/v1/file/download_info", 0)
	Mkdir          = InitApiInfo(Api+"/upload/v1/file/mkdir", 2)
	Move           = InitApiInfo(Api+"/api/v1/file/move", 1)
	Rename         = InitApiInfo(Api+"/api/v1/file/name", 1)
	Trash          = InitApiInfo(Api+"/api/v1/file/trash", 2)
	UploadCreate   = InitApiInfo(Api+"/upload/v1/file/create", 2)
	UploadUrl      = InitApiInfo(Api+"/upload/v1/file/get_upload_url", 0)
	UploadComplete = InitApiInfo(Api+"/upload/v1/file/upload_complete", 0)
	UploadAsync    = InitApiInfo(Api+"/upload/v1/file/upload_async_result", 1)
)

func (d *Open123) Request(apiInfo *ApiInfo, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	retryToken := true
	for {
		req := base.RestyClient.R()
		req.SetHeaders(map[string]string{
			"authorization": "Bearer " + d.AccessToken,
			"platform":      "open_platform",
			"Content-Type":  "application/json",
		})

		if callback != nil {
			callback(req)
		}
		if resp != nil {
			req.SetResult(resp)
		}

		log.Debugf("API: %s, QPS: %d, NowLen: %d", apiInfo.url, apiInfo.qps, apiInfo.NowLen())

		apiInfo.Require()
		defer apiInfo.Release()
		res, err := req.Execute(method, apiInfo.url)
		if err != nil {
			return nil, err
		}
		body := res.Body()

		// 解析为通用响应
		var baseResp BaseResp
		if err = json.Unmarshal(body, &baseResp); err != nil {
			return nil, err
		}

		if baseResp.Code == 0 {
			return body, nil
		} else if baseResp.Code == 401 && retryToken {
			retryToken = false
			if err := d.flushAccessToken(); err != nil {
				return nil, err
			}
		} else if baseResp.Code == 429 {
			time.Sleep(500 * time.Millisecond)
			log.Warningf("123Open: %s, QPS: %d, 请求太频繁，对应API提示过多请减小QPS", apiInfo.url, apiInfo.qps)
		} else {
			return nil, errors.New(baseResp.Message)
		}
	}

}

func (d *Open123) flushAccessToken() error {
	if d.Addition.ClientID != "" {
		if d.Addition.ClientSecret != "" {
			var resp AccessTokenResp
			_, err := d.Request(AccessToken, http.MethodPost, func(req *resty.Request) {
				req.SetBody(base.Json{
					"clientID":     d.ClientID,
					"clientSecret": d.ClientSecret,
				})
			}, &resp)
			if err != nil {
				return err
			}
			d.AccessToken = resp.Data.AccessToken
			op.MustSaveDriverStorage(d)
		} else if d.Addition.RefreshToken != "" {
			var resp RefreshTokenResp
			_, err := d.Request(RefreshToken, http.MethodPost, func(req *resty.Request) {
				req.SetQueryParam("client_id", d.ClientID)
				req.SetQueryParam("grant_type", "refresh_token")
				req.SetQueryParam("refresh_token", d.Addition.RefreshToken)
			}, &resp)
			if err != nil {
				return err
			}
			d.AccessToken = resp.AccessToken
			d.RefreshToken = resp.RefreshToken
			op.MustSaveDriverStorage(d)
		}
	}
	return nil
}

func (d *Open123) getUserInfo() (*UserInfoResp, error) {
	var resp UserInfoResp

	if _, err := d.Request(UserInfo, http.MethodGet, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (d *Open123) getFiles(parentFileId int64, limit int, lastFileId int64) (*FileListResp, error) {
	var resp FileListResp

	_, err := d.Request(FileList, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(
			map[string]string{
				"parentFileId": strconv.FormatInt(parentFileId, 10),
				"limit":        strconv.Itoa(limit),
				"lastFileId":   strconv.FormatInt(lastFileId, 10),
				"trashed":      "false",
				"searchMode":   "",
				"searchData":   "",
			})
	}, &resp)

	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (d *Open123) getDownloadInfo(fileId int64) (*DownloadInfoResp, error) {
	var resp DownloadInfoResp

	_, err := d.Request(DownloadInfo, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"fileId": strconv.FormatInt(fileId, 10),
		})
	}, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (d *Open123) mkdir(parentID int64, name string) error {
	_, err := d.Request(Mkdir, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"parentID": strconv.FormatInt(parentID, 10),
			"name":     name,
		})
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *Open123) move(fileID, toParentFileID int64) error {
	_, err := d.Request(Move, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"fileIDs":        []int64{fileID},
			"toParentFileID": toParentFileID,
		})
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *Open123) rename(fileId int64, fileName string) error {
	_, err := d.Request(Rename, http.MethodPut, func(req *resty.Request) {
		req.SetBody(base.Json{
			"fileId":   fileId,
			"fileName": fileName,
		})
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *Open123) trash(fileId int64) error {
	_, err := d.Request(Trash, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"fileIDs": []int64{fileId},
		})
	}, nil)
	if err != nil {
		return err
	}

	return nil
}
