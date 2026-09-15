package quark

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand"
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

// puusRefreshInterval __puus 有效期约 3 小时，提前定时刷新，见 AlistGo/alist#830。
// 100 分钟刷新一次，叠加 ±5 分钟（精确到秒）的随机抖动，
// 避免多个实例/账号在同一时刻集中刷新
const (
	puusRefreshInterval = 100 * time.Minute
	puusRefreshJitter   = 5 * time.Minute
)

// refreshJitter 返回 [-5min, +5min] 的随机抖动，精确到秒
func refreshJitter() time.Duration {
	seconds := rand.Int63n(2*int64(puusRefreshJitter/time.Second)+1) - int64(puusRefreshJitter/time.Second)
	return time.Duration(seconds) * time.Second
}

func (d *QuarkOrUC) request(pathname string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	d.cookieMu.Lock()
	cookieStr := d.Cookie
	d.cookieMu.Unlock()
	return d.requestWithCookie(pathname, method, callback, resp, cookieStr)
}

// requestWithCookie 使用指定的 cookie 发起请求，响应中的 __puus/__pus 会合并回 d.Cookie
func (d *QuarkOrUC) requestWithCookie(pathname string, method string, callback base.ReqCallback, resp interface{}, cookieStr string) ([]byte, error) {
	u := d.conf.api + pathname
	client := base.RestyClient
	if d.client != nil {
		client = d.client
	}
	req := client.R()
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
		return nil, err
	}
	var updated bool
	d.cookieMu.Lock()
	__puus := cookie.GetCookie(res.Cookies(), "__puus")
	if __puus != nil {
		d.Cookie = cookie.SetStr(d.Cookie, "__puus", __puus.Value)
		updated = true
	}
	if d.UseTransCodingAddress && d.config.Name == "Quark" {
		__pus := cookie.GetCookie(res.Cookies(), "__pus")
		if __pus != nil {
			d.Cookie = cookie.SetStr(d.Cookie, "__pus", __pus.Value)
			updated = true
		}
	}
	d.cookieMu.Unlock()
	if updated {
		op.MustSaveDriverStorage(d)
	}
	if e.Status >= 400 || e.Code != 0 {
		return nil, errors.New(e.Message)
	}
	return res.Body(), nil
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
	// 快照请求前的 cookie：下载 URL 的签名基于请求 /file/download 时携带的 cookie 生成，
	// 下载请求头必须与之一致，否则会被上游判定签名无效返回 403。
	// 请求和下载头使用同一个快照，避免定时刷新并发修改 d.Cookie 导致两者不一致。
	d.cookieMu.Lock()
	reqCookie := d.Cookie
	d.cookieMu.Unlock()
	_, err := d.requestWithCookie("/file/download", http.MethodPost, func(req *resty.Request) {
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
			"Cookie":     []string{reqCookie},
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

// startRefreshLoop 启动 __puus 定时刷新，保证会话 cookie 不过期
func (d *QuarkOrUC) startRefreshLoop() {
	d.refreshMu.Lock()
	defer d.refreshMu.Unlock()
	if d.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	go d.refreshLoop(ctx)
}

func (d *QuarkOrUC) refreshLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(puusRefreshInterval + refreshJitter()):
			_ = d.refreshPuus()
		}
	}
}

// maskSecret 打码敏感值，仅用于日志展示
func maskSecret(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:8] + "***"
}

// refreshPuus 发起一次不带 __puus 的请求，让服务端重新下发会话 cookie。
// 服务端只在请求缺失 __puus 字段时才更新该 cookie（见 AlistGo/alist#830）。
func (d *QuarkOrUC) refreshPuus() error {
	d.cookieMu.Lock()
	old := d.Cookie
	stripped := cookie.DelStr(old, "__puus")
	d.cookieMu.Unlock()
	_, err := d.requestWithCookie("/config", http.MethodGet, nil, nil, stripped)
	d.cookieMu.Lock()
	defer d.cookieMu.Unlock()
	if err != nil {
		// 刷新失败：仅当没有其他请求带来更新的 __puus 时才恢复旧值，
		// 避免覆盖并发请求刚合并进来的新 cookie
		if cookie.GetStr(d.Cookie, "__puus") == "" {
			d.Cookie = old
		}
		log.Warnf("quark: refresh __puus failed: %v", err)
		return err
	}
	if cookie.GetStr(d.Cookie, "__puus") == "" {
		// 服务端未重新下发：同样只在没有并发新值时恢复旧值
		d.Cookie = old
		log.Infof("quark: __puus not refreshed, server did not reissue a new value, keeping existing cookie")
		return nil
	}
	log.Infof("quark: __puus refreshed successfully: %s", maskSecret(cookie.GetStr(d.Cookie, "__puus")))
	return nil
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
