package quark

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/cookie"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

func (d *QuarkOrUC) request(pathname string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	u := d.conf.api + pathname
	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		"Cookie":  d.Cookie,
		"Accept":  "application/json, text/plain, */*",
		"Referer": d.conf.referer,
	})
	req.SetQueryParam("pr", d.conf.pr)
	req.SetQueryParam("fr", "pc")
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e Resp
	req.SetError(&e)
	res, err := req.Execute(method, u)
	if err != nil {
		return nil, err
	}
	__puus := cookie.GetCookie(res.Cookies(), "__puus")
	if __puus != nil {
		d.Cookie = cookie.SetStr(d.Cookie, "__puus", __puus.Value)
		op.MustSaveDriverStorage(d)
	}
	if e.Status >= 400 || e.Code != 0 {
		return nil, errors.New(e.Message)
	}
	return res.Body(), nil
}

func (d *QuarkOrUC) GetFiles(parent string) ([]File, error) {
	files := make([]File, 0)
	page := 1
	size := 100
	query := map[string]string{
		"pdir_fid":     parent,
		"_size":        strconv.Itoa(size),
		"_fetch_total": "1",
	}
	if d.OrderBy != "none" {
		query["_sort"] = "file_type:asc," + d.OrderBy + ":" + d.OrderDirection
	}
	for {
		query["_page"] = strconv.Itoa(page)
		var resp SortResp
		_, err := d.request("/file/sort", http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(query)
		}, &resp)
		if err != nil {
			return nil, err
		}
		files = append(files, resp.Data.List...)
		if page*size >= resp.Metadata.Total {
			break
		}
		page++
	}
	return files, nil
}

func (d *QuarkOrUC) upPre(file model.FileStreamer, parentId string) (UpPreResp, error) {
	now := time.Now()
	data := base.Json{
		"ccp_hash_update": true,
		"dir_name":        "",
		"file_name":       file.GetName(),
		"format_type":     file.GetMimetype(),
		"l_created_at":    now.UnixMilli(),
		"l_updated_at":    now.UnixMilli(),
		"pdir_fid":        parentId,
		"size":            file.GetSize(),
		//"same_path_reuse": true,
	}
	var resp UpPreResp
	_, err := d.request("/file/upload/pre", http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, &resp)
	return resp, err
}

func (d *QuarkOrUC) upHash(md5, sha1, taskId string) (bool, error) {
	data := base.Json{
		"md5":     md5,
		"sha1":    sha1,
		"task_id": taskId,
	}
	log.Debugf("hash: %+v", data)
	var resp HashResp
	_, err := d.request("/file/update/hash", http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, &resp)
	return resp.Data.Finish, err
}

func (d *QuarkOrUC) upPart(ctx context.Context, pre UpPreResp, mineType string, partNumber int, bytes io.Reader) (string, error) {
	//func (driver QuarkOrUC) UpPart(pre UpPreResp, mineType string, partNumber int, bytes []byte, account *model.Account, md5Str, sha1Str string) (string, error) {
	timeStr := time.Now().UTC().Format(http.TimeFormat)
	data := base.Json{
		"auth_info": pre.Data.AuthInfo,
		"auth_meta": fmt.Sprintf(`PUT

%s
%s
x-oss-date:%s
x-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit
/%s/%s?partNumber=%d&uploadId=%s`,
			mineType, timeStr, timeStr, pre.Data.Bucket, pre.Data.ObjKey, partNumber, pre.Data.UploadId),
		"task_id": pre.Data.TaskId,
	}
	var resp UpAuthResp
	_, err := d.request("/file/upload/auth", http.MethodPost, func(req *resty.Request) {
		req.SetBody(data).SetContext(ctx)
	}, &resp)
	if err != nil {
		return "", err
	}
	//if partNumber == 1 {
	//	finish, err := driver.UpHash(md5Str, sha1Str, pre.Data.TaskId, account)
	//	if err != nil {
	//		return "", err
	//	}
	//	if finish {
	//		return "finish", nil
	//	}
	//}
	u := fmt.Sprintf("https://%s.%s/%s", pre.Data.Bucket, pre.Data.UploadUrl[7:], pre.Data.ObjKey)
	res, err := base.RestyClient.R().SetContext(ctx).
		SetHeaders(map[string]string{
			"Authorization":    resp.Data.AuthKey,
			"Content-Type":     mineType,
			"Referer":          "https://pan.quark.cn/",
			"x-oss-date":       timeStr,
			"x-oss-user-agent": "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit",
		}).
		SetQueryParams(map[string]string{
			"partNumber": strconv.Itoa(partNumber),
			"uploadId":   pre.Data.UploadId,
		}).SetBody(bytes).Put(u)
	if err != nil {
		return "", err
	}
	if res.StatusCode() != 200 {
		return "", fmt.Errorf("up status: %d, error: %s", res.StatusCode(), res.String())
	}
	return res.Header().Get("Etag"), nil
}

func (d *QuarkOrUC) upCommit(pre UpPreResp, md5s []string) error {
	timeStr := time.Now().UTC().Format(http.TimeFormat)
	log.Debugf("md5s: %+v", md5s)
	bodyBuilder := strings.Builder{}
	bodyBuilder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
`)
	for i, m := range md5s {
		bodyBuilder.WriteString(fmt.Sprintf(`<Part>
<PartNumber>%d</PartNumber>
<ETag>%s</ETag>
</Part>
`, i+1, m))
	}
	bodyBuilder.WriteString("</CompleteMultipartUpload>")
	body := bodyBuilder.String()
	m := md5.New()
	m.Write([]byte(body))
	contentMd5 := base64.StdEncoding.EncodeToString(m.Sum(nil))
	callbackBytes, err := utils.Json.Marshal(pre.Data.Callback)
	if err != nil {
		return err
	}
	callbackBase64 := base64.StdEncoding.EncodeToString(callbackBytes)
	data := base.Json{
		"auth_info": pre.Data.AuthInfo,
		"auth_meta": fmt.Sprintf(`POST
%s
application/xml
%s
x-oss-callback:%s
x-oss-date:%s
x-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit
/%s/%s?uploadId=%s`,
			contentMd5, timeStr, callbackBase64, timeStr,
			pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadId),
		"task_id": pre.Data.TaskId,
	}
	log.Debugf("xml: %s", body)
	log.Debugf("auth data: %+v", data)
	var resp UpAuthResp
	_, err = d.request("/file/upload/auth", http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, &resp)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("https://%s.%s/%s", pre.Data.Bucket, pre.Data.UploadUrl[7:], pre.Data.ObjKey)
	res, err := base.RestyClient.R().
		SetHeaders(map[string]string{
			"Authorization":    resp.Data.AuthKey,
			"Content-MD5":      contentMd5,
			"Content-Type":     "application/xml",
			"Referer":          "https://pan.quark.cn/",
			"x-oss-callback":   callbackBase64,
			"x-oss-date":       timeStr,
			"x-oss-user-agent": "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit",
		}).
		SetQueryParams(map[string]string{
			"uploadId": pre.Data.UploadId,
		}).SetBody(body).Post(u)
	if err != nil {
		return err
	}
	if res.StatusCode() != 200 {
		return fmt.Errorf("up status: %d, error: %s", res.StatusCode(), res.String())
	}
	return nil
}

func (d *QuarkOrUC) upFinish(pre UpPreResp) error {
	data := base.Json{
		"obj_key": pre.Data.ObjKey,
		"task_id": pre.Data.TaskId,
	}
	_, err := d.request("/file/upload/finish", http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	if err != nil {
		return err
	}
	time.Sleep(time.Second)
	return nil
}
