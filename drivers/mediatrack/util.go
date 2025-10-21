package mediatrack

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

func (d *MediaTrack) request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()
	req.SetHeader("Authorization", "Bearer "+d.AccessToken)
	if d.DeviceFingerprint != "" {
		req.SetHeader("X-Device-Fingerprint", d.DeviceFingerprint)
	}
	if callback != nil {
		callback(req)
	}
	var e BaseResp
	req.SetResult(&e)
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	log.Debugln(res.String())
	if e.Status != "SUCCESS" {
		return nil, errors.New(e.Message)
	}
	if resp != nil {
		err = utils.Json.Unmarshal(res.Body(), resp)
	}
	return res.Body(), err
}

func (d *MediaTrack) getFiles(parentId string) ([]File, error) {
	files := make([]File, 0)
	url := fmt.Sprintf("https://jayce.api.mediatrack.cn/v4/assets/%s/children", parentId)
	sort := ""
	if d.OrderBy != "" {
		if d.OrderDesc {
			sort = "-"
		}
		sort += d.OrderBy
	}
	page := 1
	for {
		var resp ChildrenResp
		_, err := d.request(url, http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(map[string]string{
				"page": strconv.Itoa(page),
				"size": "50",
				"sort": sort,
			})
		}, &resp)
		if err != nil {
			return nil, err
		}
		if len(resp.Data.Assets) == 0 {
			break
		}
		page++
		files = append(files, resp.Data.Assets...)
	}
	return files, nil
}
