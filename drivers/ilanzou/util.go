package template

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/foxxorcat/mopan-sdk-go"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

func (d *ILanZou) login() error {
	res, err := d.unproved("/login", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"loginName": d.Username,
			"loginPwd":  d.Password,
		})
	})
	if err != nil {
		return err
	}
	d.Token = utils.Json.Get(res, "data", "appToken").ToString()
	if d.Token == "" {
		return fmt.Errorf("failed to login: token is empty, resp: %s", res)
	}
	return nil
}

func getTimestamp(secret []byte) (int64, string, error) {
	ts := time.Now().UnixMilli()
	tsStr := strconv.FormatInt(ts, 10)
	res, err := mopan.AesEncrypt([]byte(tsStr), secret)
	if err != nil {
		return 0, "", err
	}
	return ts, hex.EncodeToString(res), nil
}

func (d *ILanZou) request(pathname, method string, callback base.ReqCallback, proved bool, retry ...bool) ([]byte, error) {
	_, ts_str, err := getTimestamp(d.conf.secret)
	if err != nil {
		return nil, err
	}

	params := []string{
		"uuid=" + url.QueryEscape(d.UUID),
		"devType=6",
		"devCode=" + url.QueryEscape(d.UUID),
		"devModel=chrome",
		"devVersion=" + url.QueryEscape(d.conf.devVersion),
		"appVersion=",
		"timestamp=" + ts_str,
	}

	if proved {
		params = append(params, "appToken="+url.QueryEscape(d.Token))
	}

	params = append(params, "extra=2")

	queryString := strings.Join(params, "&")

	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		"Origin":          d.conf.site,
		"Referer":         d.conf.site + "/",
		"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",
		"Accept-Encoding": "gzip, deflate, br, zstd",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6,mt;q=0.5",
	})

	if d.Addition.Ip != "" {
		req.SetHeader("X-Forwarded-For", d.Addition.Ip)
	}

	if callback != nil {
		callback(req)
	}

	res, err := req.Execute(method, d.conf.base+pathname+"?"+queryString)
	if err != nil {
		if res != nil {
			log.Errorf("[iLanZou] request error: %s", res.String())
		}
		return nil, err
	}
	isRetry := len(retry) > 0 && retry[0]
	body := res.Body()
	code := utils.Json.Get(body, "code").ToInt()
	msg := utils.Json.Get(body, "msg").ToString()
	if code != 200 {
		if !isRetry && proved && (utils.SliceContains([]int{-1, -2}, code) || d.Token == "") {
			err = d.login()
			if err != nil {
				return nil, err
			}
			return d.request(pathname, method, callback, proved, true)
		}
		return nil, fmt.Errorf("%d: %s", code, msg)
	}
	return body, nil
}

func (d *ILanZou) unproved(pathname, method string, callback base.ReqCallback) ([]byte, error) {
	return d.request("/"+d.conf.unproved+pathname, method, callback, false)
}

func (d *ILanZou) proved(pathname, method string, callback base.ReqCallback) ([]byte, error) {
	return d.request("/"+d.conf.proved+pathname, method, callback, true)
}
