package baidu_netdisk

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

func (d *BaiduNetdisk) refreshToken() error {
	err := d._refreshToken()
	if err != nil && errors.Is(err, errs.EmptyToken) {
		err = d._refreshToken()
	}
	return err
}

func (d *BaiduNetdisk) _refreshToken() error {
	u := "https://openapi.baidu.com/oauth/2.0/token"
	var resp base.TokenResp
	var e TokenErrResp
	_, err := base.RestyClient.R().SetResult(&resp).SetError(&e).SetQueryParams(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": d.RefreshToken,
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
	}).Get(u)
	if err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("%s : %s", e.Error, e.ErrorDescription)
	}
	if resp.RefreshToken == "" {
		return errs.EmptyToken
	}
	d.AccessToken, d.RefreshToken = resp.AccessToken, resp.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *BaiduNetdisk) request(furl string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	var result []byte
	err := retry.Do(func() error {
		req := base.RestyClient.R()
		req.SetQueryParam("access_token", d.AccessToken)
		if callback != nil {
			callback(req)
		}
		if resp != nil {
			req.SetResult(resp)
		}
		res, err := req.Execute(method, furl)
		if err != nil {
			return err
		}
		log.Debugf("[baidu_netdisk] req: %s, resp: %s", furl, res.String())
		errno := utils.Json.Get(res.Body(), "errno").ToInt()
		if errno != 0 {
			if utils.SliceContains([]int{111, -6}, errno) {
				log.Info("refreshing baidu_netdisk token.")
				err2 := d.refreshToken()
				if err2 != nil {
					return retry.Unrecoverable(err2)
				}
			}

			if 31023 == errno && d.DownloadAPI == "crack_video" {
				result = res.Body()
				return nil
			}

			return fmt.Errorf("req: [%s] ,errno: %d, refer to https://pan.baidu.com/union/doc/", furl, errno)
		}
		result = res.Body()
		return nil
	},
		retry.LastErrorOnly(true),
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay))
	return result, err
}

func (d *BaiduNetdisk) get(pathname string, params map[string]string, resp interface{}) ([]byte, error) {
	return d.request("https://pan.baidu.com/rest/2.0"+pathname, http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(params)
	}, resp)
}

func (d *BaiduNetdisk) postForm(pathname string, params map[string]string, form map[string]string, resp interface{}) ([]byte, error) {
	return d.request("https://pan.baidu.com/rest/2.0"+pathname, http.MethodPost, func(req *resty.Request) {
		req.SetQueryParams(params)
		req.SetFormData(form)
	}, resp)
}

func (d *BaiduNetdisk) getFiles(dir string) ([]File, error) {
	start := 0
	limit := 200
	params := map[string]string{
		"method": "list",
		"dir":    dir,
		"web":    "web",
	}
	if d.OrderBy != "" {
		params["order"] = d.OrderBy
		if d.OrderDirection == "desc" {
			params["desc"] = "1"
		}
	}
	res := make([]File, 0)
	for {
		params["start"] = strconv.Itoa(start)
		params["limit"] = strconv.Itoa(limit)
		start += limit
		var resp ListResp
		_, err := d.get("/xpan/file", params, &resp)
		if err != nil {
			return nil, err
		}
		if len(resp.List) == 0 {
			break
		}

		if d.OnlyListVideoFile {
			for _, file := range resp.List {
				if file.Isdir == 1 || file.Category == 1 {
					res = append(res, file)
				}
			}
		} else {
			res = append(res, resp.List...)
		}
	}
	return res, nil
}

func (d *BaiduNetdisk) linkOfficial(file model.Obj, _ model.LinkArgs) (*model.Link, error) {
	var resp DownloadResp
	params := map[string]string{
		"method": "filemetas",
		"fsids":  fmt.Sprintf("[%s]", file.GetID()),
		"dlink":  "1",
	}
	_, err := d.get("/xpan/multimedia", params, &resp)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s&access_token=%s", resp.List[0].Dlink, d.AccessToken)
	res, err := base.NoRedirectClient.R().SetHeader("User-Agent", "pan.baidu.com").Head(u)
	if err != nil {
		return nil, err
	}
	//if res.StatusCode() == 302 {
	u = res.Header().Get("location")
	//}

	return &model.Link{
		URL: u,
		Header: http.Header{
			"User-Agent": []string{"pan.baidu.com"},
		},
	}, nil
}

func (d *BaiduNetdisk) linkCrack(file model.Obj, _ model.LinkArgs) (*model.Link, error) {
	var resp DownloadResp2
	param := map[string]string{
		"target": fmt.Sprintf("[\"%s\"]", file.GetPath()),
		"dlink":  "1",
		"web":    "5",
		"origin": "dlna",
	}
	_, err := d.request("https://pan.baidu.com/api/filemetas", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(param)
	}, &resp)
	if err != nil {
		return nil, err
	}

	return &model.Link{
		URL: resp.Info[0].Dlink,
		Header: http.Header{
			"User-Agent": []string{d.CustomCrackUA},
		},
	}, nil
}

func (d *BaiduNetdisk) linkCrackVideo(file model.Obj, _ model.LinkArgs) (*model.Link, error) {
	param := map[string]string{
		"type":       "VideoURL",
		"path":       fmt.Sprintf("%s", file.GetPath()),
		"fs_id":      file.GetID(),
		"devuid":     "0%1",
		"clienttype": "1",
		"channel":    "android_15_25010PN30C_bd-netdisk_1523a",
		"nom3u8":     "1",
		"dlink":      "1",
		"media":      "1",
		"origin":     "dlna",
	}
	resp, err := d.request("https://pan.baidu.com/api/mediainfo", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(param)
	}, nil)
	if err != nil {
		return nil, err
	}

	return &model.Link{
		URL: utils.Json.Get(resp, "info", "dlink").ToString(),
		Header: http.Header{
			"User-Agent": []string{d.CustomCrackUA},
		},
	}, nil
}

func (d *BaiduNetdisk) manage(opera string, filelist any) ([]byte, error) {
	params := map[string]string{
		"method": "filemanager",
		"opera":  opera,
	}
	marshal, _ := utils.Json.MarshalToString(filelist)
	return d.postForm("/xpan/file", params, map[string]string{
		"async":    "0",
		"filelist": marshal,
		"ondup":    "fail",
	}, nil)
}

func (d *BaiduNetdisk) create(path string, size int64, isdir int, uploadid, block_list string, resp any, mtime, ctime int64) ([]byte, error) {
	params := map[string]string{
		"method": "create",
	}
	form := map[string]string{
		"path":  path,
		"size":  strconv.FormatInt(size, 10),
		"isdir": strconv.Itoa(isdir),
		"rtype": "3",
	}
	if mtime != 0 && ctime != 0 {
		joinTime(form, ctime, mtime)
	}

	if uploadid != "" {
		form["uploadid"] = uploadid
	}
	if block_list != "" {
		form["block_list"] = block_list
	}
	return d.postForm("/xpan/file", params, form, resp)
}

func joinTime(form map[string]string, ctime, mtime int64) {
	form["local_mtime"] = strconv.FormatInt(mtime, 10)
	form["local_ctime"] = strconv.FormatInt(ctime, 10)
}

const (
	DefaultSliceSize int64 = 4 * utils.MB
	VipSliceSize     int64 = 16 * utils.MB
	SVipSliceSize    int64 = 32 * utils.MB

	MaxSliceNum       = 2048 // 文档写的是 1024/没写 ，但实际测试是 2048
	SliceStep   int64 = 1 * utils.MB
)

func (d *BaiduNetdisk) getSliceSize(filesize int64) int64 {
	// 非会员固定为 4MB
	if d.vipType == 0 {
		if d.CustomUploadPartSize != 0 {
			log.Warnf("CustomUploadPartSize is not supported for non-vip user, use DefaultSliceSize")
		}
		if filesize > MaxSliceNum*DefaultSliceSize {
			log.Warnf("File size(%d) is too large, may cause upload failure", filesize)
		}

		return DefaultSliceSize
	}

	if d.CustomUploadPartSize != 0 {
		if d.CustomUploadPartSize < DefaultSliceSize {
			log.Warnf("CustomUploadPartSize(%d) is less than DefaultSliceSize(%d), use DefaultSliceSize", d.CustomUploadPartSize, DefaultSliceSize)
			return DefaultSliceSize
		}

		if d.vipType == 1 && d.CustomUploadPartSize > VipSliceSize {
			log.Warnf("CustomUploadPartSize(%d) is greater than VipSliceSize(%d), use VipSliceSize", d.CustomUploadPartSize, VipSliceSize)
			return VipSliceSize
		}

		if d.vipType == 2 && d.CustomUploadPartSize > SVipSliceSize {
			log.Warnf("CustomUploadPartSize(%d) is greater than SVipSliceSize(%d), use SVipSliceSize", d.CustomUploadPartSize, SVipSliceSize)
			return SVipSliceSize
		}

		return d.CustomUploadPartSize
	}

	maxSliceSize := DefaultSliceSize

	switch d.vipType {
	case 1:
		maxSliceSize = VipSliceSize
	case 2:
		maxSliceSize = SVipSliceSize
	}

	// upload on low bandwidth
	if d.LowBandwithUploadMode {
		size := DefaultSliceSize

		for size <= maxSliceSize {
			if filesize <= MaxSliceNum*size {
				return size
			}

			size += SliceStep
		}
	}

	if filesize > MaxSliceNum*maxSliceSize {
		log.Warnf("File size(%d) is too large, may cause upload failure", filesize)
	}

	return maxSliceSize
}

// func encodeURIComponent(str string) string {
// 	r := url.QueryEscape(str)
// 	r = strings.ReplaceAll(r, "+", "%20")
// 	return r
// }

func DecryptMd5(encryptMd5 string) string {
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

func EncryptMd5(originalMd5 string) string {
	reversed := originalMd5[8:16] + originalMd5[:8] + originalMd5[24:32] + originalMd5[16:24]

	var out strings.Builder
	out.Grow(len(reversed))
	for i, n := 0, int64(0); i < len(reversed); i++ {
		n, _ = strconv.ParseInt(reversed[i:i+1], 16, 64)
		n ^= int64(15 & i)
		if i == 9 {
			out.WriteRune(rune(n) + 'g')
		} else {
			out.WriteString(strconv.FormatInt(n, 16))
		}
	}
	return out.String()
}
