package quark

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
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
	d.cookieMu.Lock()
	cookieStr := d.Cookie
	d.cookieMu.Unlock()
	body, _, err := d.requestWithCookie(pathname, method, callback, resp, cookieStr)
	return body, err
}

func (d *QuarkOrUC) requestClient() *resty.Client {
	d.cookieMu.Lock()
	defer d.cookieMu.Unlock()
	if d.client == nil {
		// Cookie 由驱动显式管理，禁用自动缓存以免追加旧值。
		d.client = base.NewRestyClient().SetCookieJar(nil)
	}
	return d.client
}

func mergeResponseCookies(base string, responseCookies []*http.Cookie) (string, bool) {
	if cookie.GetCookie(responseCookies, "__puus") == nil {
		return base, false
	}
	merged := base
	for _, c := range responseCookies {
		if c.Name != "" {
			merged = cookie.SetStr(merged, c.Name, c.Value)
		}
	}
	return merged, true
}

func (d *QuarkOrUC) mergeResponseCookies(res *resty.Response) {
	if res == nil {
		return
	}
	d.cookieMu.Lock()
	defer d.cookieMu.Unlock()
	merged, updated := mergeResponseCookies(d.Cookie, res.Cookies())
	if !updated && d.UseTransCodingAddress && d.config.Name == "Quark" {
		__pus := cookie.GetCookie(res.Cookies(), "__pus")
		if __pus != nil {
			merged = cookie.SetStr(merged, "__pus", __pus.Value)
			updated = true
		}
	}
	if updated {
		d.Cookie = merged
		op.MustSaveDriverStorage(d)
	}
}

// preRequestClient returns a PRE-only Resty wrapper that reuses the selected
// client's underlying transport, cookie jar and timeout but disables both Resty
// retries and HTTP redirects. /file/upload/pre may allocate a task before its
// response is lost, so neither retry nor redirect replay is safe here.
func (d *QuarkOrUC) preRequestClient() *resty.Client {
	source := d.requestClient()
	sourceHTTP := source.GetClient()
	httpClient := &http.Client{
		Transport: sourceHTTP.Transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar:     sourceHTTP.Jar,
		Timeout: sourceHTTP.Timeout,
	}
	client := resty.NewWithClient(httpClient).SetRetryCount(0)
	client.Header = source.Header.Clone()
	client.JSONMarshal = source.JSONMarshal
	client.JSONUnmarshal = source.JSONUnmarshal
	client.XMLMarshal = source.XMLMarshal
	client.XMLUnmarshal = source.XMLUnmarshal
	return client
}

// requestWithCookie 使用指定 Cookie 发起请求，返回本次响应合并后的 Cookie
func (d *QuarkOrUC) requestWithCookie(pathname string, method string, callback base.ReqCallback, resp interface{}, cookieStr string) ([]byte, string, error) {
	u := d.conf.api + pathname
	req := d.requestClient().R()
	req.SetHeaders(map[string]string{
		"Cookie":  cookieStr,
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
		return nil, cookieStr, err
	}
	responseCookie, _ := mergeResponseCookies(cookieStr, res.Cookies())
	d.mergeResponseCookies(res)
	if e.Status >= 400 || e.Code != 0 {
		return nil, responseCookie, &providerError{
			HTTPStatus: res.StatusCode(),
			Status:     e.Status,
			Code:       e.Code,
			Message:    e.Message,
		}
	}
	return res.Body(), responseCookie, nil
}

func (d *QuarkOrUC) GetFiles(parent string) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	page := 1
	size := 100
	query := map[string]string{
		"pdir_fid":             parent,
		"_size":                strconv.Itoa(size),
		"_fetch_total":         "1",
		"fetch_all_file":       "1",
		"fetch_risk_file_name": "1",
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
		for _, file := range resp.Data.List {
			file.FileName = html.UnescapeString(file.FileName)
			if d.OnlyListVideoFile {
				if file.IsDir() || file.Category == 1 {
					files = append(files, &file)
				}
			} else {
				files = append(files, &file)
			}
		}
		if page*size >= resp.Metadata.Total {
			break
		}
		page++
	}
	return files, nil
}

func (d *QuarkOrUC) getDownloadLink(file model.Obj) (*model.Link, error) {
	var ut string
	if d.config.Name == "UC" {
		var err error
		ut, err = ucDownloadToken(d.UTDID)
		if err != nil {
			return nil, err
		}
	}
	data := base.Json{
		"fids": []string{file.GetID()},
	}
	var resp DownResp
	ua := d.conf.ua
	// 下载 URL 使用本次响应合并后的 Cookie，避免并发请求错配。
	d.cookieMu.Lock()
	reqCookie := d.Cookie
	d.cookieMu.Unlock()
	_, downloadCookie, err := d.requestWithCookie("/file/download", http.MethodPost, func(req *resty.Request) {
		if ut != "" {
			req.SetQueryParam("ut", ut)
		}
		req.SetHeader("User-Agent", ua).
			SetBody(data)
	}, &resp, reqCookie)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 || resp.Data[0].DownloadUrl == "" {
		return nil, errors.New("no download link found")
	}
	link := &model.Link{
		URL: resp.Data[0].DownloadUrl,
		Header: http.Header{
			"Cookie":     []string{downloadCookie},
			"Referer":    []string{d.conf.referer},
			"User-Agent": []string{ua},
		},
	}
	d.applyLinkLimit(link)
	return link, nil
}

// applyLinkLimit 按存储配置设置分片下载参数，DownConcurrency 为 0 时不强制分片下载
func (d *QuarkOrUC) applyLinkLimit(link *model.Link) {
	if d.DownConcurrency <= 0 {
		return
	}
	partSize := d.DownPartSize
	if partSize <= 0 {
		partSize = 10
	}
	link.Concurrency = d.DownConcurrency
	link.PartSize = partSize * utils.MB
}

func (d *QuarkOrUC) getTranscodingLink(file model.Obj) (*model.Link, error) {
	data := base.Json{
		"fid":         file.GetID(),
		"resolutions": "low,normal,high,super,2k,4k",
		"supports":    "fmp4_av,m3u8,dolby_vision",
	}
	var resp TranscodingResp
	ua := d.conf.ua

	_, err := d.request("/file/v2/play/project", http.MethodPost, func(req *resty.Request) {
		req.SetHeader("User-Agent", ua).
			SetBody(data)
	}, &resp)
	if err != nil {
		return nil, err
	}

	for _, info := range resp.Data.VideoList {
		if info.VideoInfo.URL != "" {
			link := &model.Link{
				URL: info.VideoInfo.URL,
			}
			d.applyLinkLimit(link)
			return link, nil
		}
	}

	return nil, errors.New("no link found")
}

func (d *QuarkOrUC) upPre(ctx context.Context, file model.FileStreamer, parentId string) (UpPreResp, error) {
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

	d.cookieMu.Lock()
	cookieStr := d.Cookie
	d.cookieMu.Unlock()

	var resp UpPreResp
	var e Resp
	req := d.preRequestClient().R().
		SetContext(ctx).
		SetHeaders(map[string]string{
			"Cookie":  cookieStr,
			"Accept":  "application/json, text/plain, */*",
			"Referer": d.conf.referer,
		}).
		SetQueryParam("pr", d.conf.pr).
		SetQueryParam("fr", "pc").
		SetBody(data).
		SetResult(&resp).
		SetError(&e)

	res, err := req.Post(d.conf.api + "/file/upload/pre")
	if err != nil {
		return resp, err
	}
	d.mergeResponseCookies(res)

	if res.StatusCode() != http.StatusOK {
		if e.Message != "" {
			return resp, errors.New(e.Message)
		}
		return resp, fmt.Errorf("unexpected HTTP status %d", res.StatusCode())
	}
	return resp, nil
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
