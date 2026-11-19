package baidu_youth

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	stdpath "path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	panBaseURL      = "https://pan.baidu.com"
	panReferer      = panBaseURL + "/"
	panUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	youthReferer    = panBaseURL + "/youth/pan/main#/index?category=all"
	youthAppID      = "250528"
	uploadAppID     = "25179614"
	videoAPIChannel = "android_15_25010PN30C_bd-netdisk_1523a"
	videoAPIDevUID  = "0%1"
)

func (d *BaiduYouth) normalizeURL(furl string) string {
	if strings.HasPrefix(furl, "http://") || strings.HasPrefix(furl, "https://") {
		return furl
	}
	return panBaseURL + furl
}

func (d *BaiduYouth) commonHeaders() map[string]string {
	return map[string]string{
		"Accept":           "application/json, text/plain, */*",
		"Accept-Language":  "zh-CN,zh;q=0.9,en;q=0.8",
		"Cache-Control":    "no-cache",
		"Cookie":           d.Cookie,
		"Origin":           panBaseURL,
		"Pragma":           "no-cache",
		"Referer":          youthReferer,
		"User-Agent":       panUserAgent,
		"X-Requested-With": "XMLHttpRequest",
	}
}

func youthQueryParams() map[string]string {
	return map[string]string{
		"app_id":     youthAppID,
		"clienttype": "0",
		"web":        "1",
	}
}

func youthUploadQueryParams() map[string]string {
	return map[string]string{
		"app_id":     uploadAppID,
		"channel":    "chunlei",
		"clienttype": "0",
		"web":        "1",
	}
}

func extractBaiduMessage(body []byte) string {
	for _, path := range [][]interface{}{
		{"show_msg"},
		{"errmsg"},
		{"error_msg"},
		{"error_description"},
		{"message"},
	} {
		msg := utils.Json.Get(body, path...).ToString()
		if msg != "" {
			return msg
		}
	}
	return ""
}

func (d *BaiduYouth) doRequest(furl string, method string, defaultQuery map[string]string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R().SetHeaders(d.commonHeaders())
	if defaultQuery != nil {
		req.SetQueryParams(defaultQuery)
	}
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	res, err := req.Execute(method, d.normalizeURL(furl))
	if err != nil {
		return nil, err
	}
	body := res.Body()
	if errno := utils.Json.Get(body, "errno").ToInt(); errno != 0 {
		msg := extractBaiduMessage(body)
		if errno == -6 && msg == "" {
			msg = "cookie expired or invalid"
		}
		if msg == "" {
			msg = "request failed"
		}
		return nil, fmt.Errorf("[baidu_youth] %s (errno=%d)", msg, errno)
	}
	return body, nil
}

func (d *BaiduYouth) get(ctx context.Context, pathname string, params map[string]string, resp interface{}) ([]byte, error) {
	return d.doRequest(pathname, http.MethodGet, youthQueryParams(), func(req *resty.Request) {
		req.SetContext(ctx)
		if params != nil {
			req.SetQueryParams(params)
		}
	}, resp)
}

func (d *BaiduYouth) postForm(ctx context.Context, pathname string, params map[string]string, form map[string]string, resp interface{}) ([]byte, error) {
	return d.doRequest(pathname, http.MethodPost, youthQueryParams(), func(req *resty.Request) {
		req.SetContext(ctx)
		if params != nil {
			req.SetQueryParams(params)
		}
		req.SetFormData(form)
	}, resp)
}

func (d *BaiduYouth) getUpload(ctx context.Context, pathname string, params map[string]string, resp interface{}) ([]byte, error) {
	return d.doRequest(pathname, http.MethodGet, youthUploadQueryParams(), func(req *resty.Request) {
		req.SetContext(ctx)
		if params != nil {
			req.SetQueryParams(params)
		}
	}, resp)
}

func (d *BaiduYouth) postUploadForm(ctx context.Context, pathname string, params map[string]string, form map[string]string, resp interface{}) ([]byte, error) {
	return d.doRequest(pathname, http.MethodPost, youthUploadQueryParams(), func(req *resty.Request) {
		req.SetContext(ctx)
		if params != nil {
			req.SetQueryParams(params)
		}
		req.SetFormData(form)
	}, resp)
}

func (d *BaiduYouth) getBare(ctx context.Context, pathname string, params map[string]string, resp interface{}) ([]byte, error) {
	return d.doRequest(pathname, http.MethodGet, nil, func(req *resty.Request) {
		req.SetContext(ctx)
		if params != nil {
			req.SetQueryParams(params)
		}
	}, resp)
}

func (d *BaiduYouth) getUserSK(ctx context.Context) (string, error) {
	body, err := d.get(ctx, "/youth/api/report/user", map[string]string{
		"action":    "sapi_auth",
		"timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10),
	}, nil)
	if err != nil {
		return "", err
	}
	return utils.Json.Get(body, "uinfo").ToString(), nil
}

func (d *BaiduYouth) getUserSession(ctx context.Context) (int64, string, string, error) {
	body, err := d.get(ctx, "/youth/api/user/getinfo", map[string]string{
		"need_selfinfo": "1",
	}, nil)
	if err != nil {
		return 0, "", "", err
	}
	uk := int64(utils.Json.Get(body, "records", 0, "uk").ToInt())
	bdstoken := utils.Json.Get(body, "records", 0, "bdstoken").ToString()
	sk := utils.Json.Get(body, "records", 0, "sk").ToString()
	if bdstoken == "" || uk == 0 || sk == "" {
		body, err = d.getBare(ctx, "/api/gettemplatevariable", map[string]string{
			"fields": `["bdstoken","uk","sk"]`,
		}, nil)
		if err != nil {
			return 0, "", "", err
		}
		if uk == 0 {
			uk = int64(utils.Json.Get(body, "result", "uk").ToInt())
		}
		if bdstoken == "" {
			bdstoken = utils.Json.Get(body, "result", "bdstoken").ToString()
		}
		if sk == "" {
			sk = utils.Json.Get(body, "result", "sk").ToString()
		}
	}
	if sk == "" {
		sk, _ = d.getUserSK(ctx)
	}
	if bdstoken == "" {
		return 0, "", "", fmt.Errorf("failed to get bdstoken from baidu youth cookie")
	}
	if uk == 0 {
		return 0, "", "", fmt.Errorf("failed to get uk from baidu youth cookie")
	}
	return uk, bdstoken, sk, nil
}

func (d *BaiduYouth) getFiles(ctx context.Context, dir string) ([]File, error) {
	page := 1
	num := 1000
	params := map[string]string{
		"dir": dir,
	}
	if d.OrderBy != "" {
		params["order"] = d.OrderBy
		if d.OrderDirection == "desc" {
			params["desc"] = "1"
		} else {
			params["desc"] = "0"
		}
	}
	files := make([]File, 0)
	for {
		params["page"] = strconv.Itoa(page)
		params["num"] = strconv.Itoa(num)
		var resp ListResp
		_, err := d.get(ctx, "/youth/api/list", params, &resp)
		if err != nil {
			return nil, err
		}
		if len(resp.List) == 0 {
			return files, nil
		}
		files = append(files, resp.List...)
		if len(resp.List) < num {
			return files, nil
		}
		page++
	}
}

func (d *BaiduYouth) getFileByPath(ctx context.Context, path string) (File, error) {
	if path == "/" {
		return File{
			Path:           "/",
			ServerFilename: "/",
			Isdir:          1,
		}, nil
	}
	target, err := utils.Json.MarshalToString([]string{path})
	if err != nil {
		return File{}, err
	}
	var resp FileMetaResp
	_, err = d.get(ctx, "/api/filemetas", map[string]string{
		"target": target,
	}, &resp)
	if err != nil {
		return File{}, err
	}
	if len(resp.Info) > 0 {
		return resp.Info[0], nil
	}
	if len(resp.List) > 0 {
		return resp.List[0], nil
	}
	return File{}, errs.NewErr(errs.ObjectNotFound, "baidu youth path not found: %s", path)
}

func (d *BaiduYouth) getByPath(ctx context.Context, path string) (model.Obj, error) {
	if path == "/" {
		return &model.Object{
			Path:     "/",
			Name:     "/",
			IsFolder: true,
		}, nil
	}
	file, err := d.getFileByPath(ctx, path)
	if err != nil {
		return nil, err
	}
	return fileToObj(file), nil
}

func (d *BaiduYouth) linkOfficial(ctx context.Context, file model.Obj) (*model.Link, error) {
	return d.buildDownloadLink(ctx, file)
}

func (d *BaiduYouth) linkCrack(ctx context.Context, file model.Obj) (*model.Link, error) {
	return d.buildDownloadLink(ctx, file)
}

func (d *BaiduYouth) downloadHeaders() http.Header {
	return http.Header{
		"Accept":          []string{"*/*"},
		"Accept-Language": []string{"zh-CN,zh;q=0.9,en;q=0.8"},
		"Cache-Control":   []string{"no-cache"},
		"Pragma":          []string{"no-cache"},
		"Referer":         []string{panReferer},
		"User-Agent":      []string{panUserAgent},
	}
}

func nextDPLogID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func (d *BaiduYouth) getCurrentUserSK(ctx context.Context) (string, error) {
	sk, err := d.getUserSK(ctx)
	if err == nil && sk != "" {
		return sk, nil
	}
	if d.sk != "" {
		return d.sk, nil
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("baidu youth sk is empty")
}

func (d *BaiduYouth) locatedownloadRand(sk string, nowMilli int64) string {
	sum := sha1.Sum([]byte(strconv.FormatInt(d.uk, 10) + sk + strconv.FormatInt(nowMilli, 10) + "0"))
	return hex.EncodeToString(sum[:])
}

func (d *BaiduYouth) locatedownloadSign(fileMD5 string, fileID string, nowMilli int64) string {
	sum := md5.Sum([]byte(fileMD5 + "_" + strconv.FormatInt(d.uk, 10) + "_" + fileID + "_" + strconv.FormatInt(nowMilli, 10)))
	return hex.EncodeToString(sum[:])
}

func (d *BaiduYouth) resolveDownloadMeta(ctx context.Context, file model.Obj) (string, string, string, error) {
	parentDir := stdpath.Dir(file.GetPath())
	if parentDir == "." {
		parentDir = "/"
	}

	files, err := d.getFiles(ctx, parentDir)
	if err != nil {
		return "", "", "", err
	}
	for _, listedFile := range files {
		if listedFile.Path != file.GetPath() {
			continue
		}
		fileID := strconv.FormatInt(listedFile.FsId, 10)
		if listedFile.Path != "" && fileID != "" && listedFile.Md5 != "" {
			return listedFile.Path, fileID, listedFile.Md5, nil
		}
		return "", "", "", fmt.Errorf("baidu youth list metadata incomplete for %s", file.GetPath())
	}
	return "", "", "", errs.NewErr(errs.ObjectNotFound, "baidu youth list metadata not found: %s", file.GetPath())
}

func normalizeLocatedownloadURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("baidu youth locatedownload url is empty")
	}
	if !strings.Contains(rawURL, "response-cache-control=") {
		sep := "&"
		if !strings.Contains(rawURL, "?") {
			sep = "?"
		}
		rawURL += sep + "response-cache-control=private"
	}
	return rawURL, nil
}

func (d *BaiduYouth) getMediaInfoDLink(ctx context.Context, file model.Obj) (string, error) {
	path, fileID, _, err := d.resolveDownloadMeta(ctx, file)
	if err != nil {
		return "", err
	}

	var resp MediaInfoResp
	_, err = d.doRequest("/youth/api/mediainfo", http.MethodGet, nil, func(req *resty.Request) {
		req.SetContext(ctx)
		req.SetQueryParams(map[string]string{
			"channel":    videoAPIChannel,
			"clienttype": "1",
			"devuid":     videoAPIDevUID,
			"dlink":      "1",
			"fs_id":      fileID,
			"media":      "1",
			"nom3u8":     "1",
			"origin":     "dlna",
			"path":       path,
			"type":       "VideoURL",
		})
	}, &resp)
	if err != nil {
		return "", err
	}
	if resp.Info.Dlink == "" {
		return "", fmt.Errorf("baidu youth video dlink not found for %s", path)
	}
	return resp.Info.Dlink, nil
}

func (d *BaiduYouth) buildVideoLink(ctx context.Context, file model.Obj) (*model.Link, error) {
	dlink, err := d.getMediaInfoDLink(ctx, file)
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL: dlink,
		Header: http.Header{
			"Referer": []string{panReferer},
		},
	}, nil
}

func (d *BaiduYouth) requestLocateDownloadURL(ctx context.Context, path string, fileID string, fileMD5 string, sk string) (string, error) {
	nowMilli := time.Now().UnixMilli()

	var resp LocateDownloadResp
	_, err := d.get(ctx, "/youth/api/locatedownload", map[string]string{
		"devuid":   "0",
		"dp-logid": nextDPLogID(),
		"path":     path,
		"rand":     d.locatedownloadRand(sk, nowMilli),
		"sign":     d.locatedownloadSign(fileMD5, fileID, nowMilli),
		"time":     strconv.FormatInt(nowMilli, 10),
	}, &resp)
	if err != nil {
		return "", err
	}
	if resp.URL == "" {
		return "", fmt.Errorf("baidu youth locatedownload url not found for %s", path)
	}
	return normalizeLocatedownloadURL(resp.URL)
}

func (d *BaiduYouth) getLocateDownloadURL(ctx context.Context, file model.Obj) (string, error) {
	path, fileID, fileMD5, err := d.resolveDownloadMeta(ctx, file)
	if err != nil {
		return "", err
	}

	sk, err := d.getCurrentUserSK(ctx)
	if err != nil {
		return "", err
	}
	downloadURL, err := d.requestLocateDownloadURL(ctx, path, fileID, fileMD5, sk)
	if err == nil {
		return downloadURL, nil
	}

	if !strings.Contains(err.Error(), "errno=-30006") {
		return "", err
	}
	sk, refreshErr := d.getUserSK(ctx)
	if refreshErr != nil {
		return "", refreshErr
	}
	if sk == "" {
		return "", err
	}
	return d.requestLocateDownloadURL(ctx, path, fileID, fileMD5, sk)
}

func (d *BaiduYouth) buildDownloadLink(ctx context.Context, file model.Obj) (*model.Link, error) {
	downloadURL, err := d.getLocateDownloadURL(ctx, file)
	if err != nil {
		return nil, err
	}
	return &model.Link{
		URL:    downloadURL,
		Header: d.downloadHeaders(),
	}, nil
}

func (d *BaiduYouth) manage(ctx context.Context, opera string, filelist any) ([]byte, error) {
	marshal, err := utils.Json.MarshalToString(filelist)
	if err != nil {
		return nil, err
	}
	return d.postForm(ctx, "/youth/api/filemanager", map[string]string{
		"async":    "0",
		"bdstoken": d.bdstoken,
		"onnest":   "fail",
		"opera":    opera,
	}, map[string]string{
		"filelist": marshal,
		"ondup":    "fail",
	}, nil)
}

func (d *BaiduYouth) precreate(ctx context.Context, path string, streamSize int64, blockListStr, contentMd5, sliceMd5 string, ctime, mtime int64) (*PrecreateResp, error) {
	form := map[string]string{
		"autoinit":    "1",
		"block_list":  blockListStr,
		"isdir":       "0",
		"path":        path,
		"size":        strconv.FormatInt(streamSize, 10),
		"target_path": stdpath.Dir(path),
	}
	if contentMd5 != "" {
		form["content-md5"] = contentMd5
	}
	if sliceMd5 != "" {
		form["slice-md5"] = sliceMd5
	}
	joinTime(form, ctime, mtime)
	var resp PrecreateResp
	_, err := d.postUploadForm(ctx, "/youth/api/precreate", map[string]string{
		"bdstoken": d.bdstoken,
	}, form, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *BaiduYouth) createFile(ctx context.Context, path, targetPath string, size int64, uploadID, uploadSign, blockList string, resp interface{}, mtime, ctime int64) ([]byte, error) {
	form := map[string]string{
		"block_list":  blockList,
		"path":        path,
		"size":        strconv.FormatInt(size, 10),
		"target_path": targetPath,
		"uploadid":    uploadID,
	}
	if uploadSign != "" {
		form["uploadsign"] = uploadSign
	}
	joinTime(form, ctime, mtime)
	return d.postUploadForm(ctx, "/youth/api/create", map[string]string{
		"bdstoken": d.bdstoken,
		"isdir":    "0",
	}, form, resp)
}

func joinTime(form map[string]string, ctime, mtime int64) {
	if ctime != 0 {
		form["local_ctime"] = strconv.FormatInt(ctime, 10)
	}
	if mtime != 0 {
		form["local_mtime"] = strconv.FormatInt(mtime, 10)
	}
}

func (d *BaiduYouth) uploadSlice(ctx context.Context, params map[string]string, fileName string, file io.Reader) error {
	res, err := d.upClient.R().
		SetContext(ctx).
		SetHeaders(d.commonHeaders()).
		SetQueryParams(youthUploadQueryParams()).
		SetQueryParams(params).
		SetFileReader("file", fileName, file).
		Post(d.UploadAPI + "/rest/2.0/pcs/superfile2")
	if err != nil {
		return err
	}
	body := res.Body()
	errCode := utils.Json.Get(body, "error_code").ToInt()
	errNo := utils.Json.Get(body, "errno").ToInt()
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "uploadid") && (strings.Contains(lower, "invalid") || strings.Contains(lower, "expired") || strings.Contains(lower, "not found")) {
		return ErrUploadIDExpired
	}
	if errCode != 0 || errNo != 0 {
		msg := extractBaiduMessage(body)
		if msg == "" {
			msg = "error uploading to baidu youth"
		}
		return errs.NewErr(errs.StreamIncomplete, "%s: %s", msg, string(body))
	}
	return nil
}

func (d *BaiduYouth) uploadProgressKey() string {
	if d.uk != 0 {
		return strconv.FormatInt(d.uk, 10)
	}
	sum := md5.Sum([]byte(d.Cookie))
	return hex.EncodeToString(sum[:])
}

func DecryptMd5(encryptMd5 string) string {
	if encryptMd5 == "" {
		return ""
	}
	if _, err := hex.DecodeString(encryptMd5); err == nil {
		return encryptMd5
	}

	var out strings.Builder
	out.Grow(len(encryptMd5))
	for i, n := 0, int64(0); i < len(encryptMd5); i++ {
		if i == 9 {
			n = int64(unicode.ToLower(rune(encryptMd5[i])) - 'g')
		} else {
			n, _ = strconv.ParseInt(encryptMd5[i:i+1], 16, 64)
		}
		out.WriteString(strconv.FormatInt(n^int64(15&i), 16))
	}

	encryptMd5 = out.String()
	return encryptMd5[8:16] + encryptMd5[:8] + encryptMd5[24:32] + encryptMd5[16:24]
}
