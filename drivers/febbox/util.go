package febbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/go-resty/resty/v2"
	"net/http"
	"strconv"
)

func (d *FebBox) refreshTokenByOAuth2() error {
	token, err := d.oauth2Token.Token()
	if err != nil {
		return err
	}
	d.Status = "work"
	d.accessToken = token.AccessToken
	d.Addition.RefreshToken = token.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *FebBox) request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()
	// 使用oauth2 获取 access_token
	token, err := d.oauth2Token.Token()
	if err != nil {
		return nil, err
	}
	req.SetAuthScheme(token.TokenType).SetAuthToken(token.AccessToken)

	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e ErrResp
	req.SetError(&e)
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}

	switch e.ErrorCode {
	case 0:
		return res.Body(), nil
	case 1:
		return res.Body(), nil
	case -10001:
		if e.ServerName != "" {
			// access_token 过期
			if err = d.refreshTokenByOAuth2(); err != nil {
				return nil, err
			}
			return d.request(url, method, callback, resp)
		} else {
			return nil, errors.New(e.Error())
		}
	default:
		return nil, errors.New(e.Error())
	}
}

func (d *FebBox) getFilesList(id string) ([]File, error) {
	if d.PageSize <= 0 {
		d.PageSize = 100
	}
	res, err := d.listWithLimit(id, d.PageSize)
	if err != nil {
		return nil, err
	}
	return *res, nil
}

func (d *FebBox) listWithLimit(dirID string, pageLimit int64) (*[]File, error) {
	var files []File
	page := int64(1)
	for {
		result, err := d.getFiles(dirID, page, pageLimit)
		if err != nil {
			return nil, err
		}
		files = append(files, *result...)
		if int64(len(*result)) < pageLimit {
			break
		} else {
			page++
		}
	}
	return &files, nil
}

func (d *FebBox) getFiles(dirID string, page, pageLimit int64) (*[]File, error) {
	var fileList FileListResp
	queryParams := map[string]string{
		"module":    "file_list",
		"parent_id": dirID,
		"page":      strconv.FormatInt(page, 10),
		"pagelimit": strconv.FormatInt(pageLimit, 10),
		"order":     d.Addition.SortRule,
	}

	res, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, &fileList)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(res, &fileList); err != nil {
		return nil, err
	}

	return &fileList.Data.FileList, nil
}

func (d *FebBox) getDownloadLink(id string, ip string) (string, error) {
	var fileDownloadResp FileDownloadResp
	queryParams := map[string]string{
		"module": "file_get_download_url",
		"fids[]": id,
		"ip":     ip,
	}

	res, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, &fileDownloadResp)
	if err != nil {
		return "", err
	}

	if err = json.Unmarshal(res, &fileDownloadResp); err != nil {
		return "", err
	}
	if len(fileDownloadResp.Data) == 0 {
		return "", fmt.Errorf("can not get download link, code:%d, msg:%s", fileDownloadResp.Code, fileDownloadResp.Msg)
	}

	return fileDownloadResp.Data[0].DownloadURL, nil
}

func (d *FebBox) makeDir(id string, name string) error {
	queryParams := map[string]string{
		"module":    "create_dir",
		"parent_id": id,
		"name":      name,
	}

	_, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *FebBox) move(id string, id2 string) error {
	queryParams := map[string]string{
		"module": "file_move",
		"fids[]": id,
		"to":     id2,
	}

	_, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *FebBox) rename(id string, name string) error {
	queryParams := map[string]string{
		"module": "file_rename",
		"fid":    id,
		"name":   name,
	}

	_, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *FebBox) copy(id string, id2 string) error {
	queryParams := map[string]string{
		"module": "file_copy",
		"fids[]": id,
		"to":     id2,
	}

	_, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, nil)
	if err != nil {
		return err
	}

	return nil
}

func (d *FebBox) remove(id string) error {
	queryParams := map[string]string{
		"module": "file_delete",
		"fids[]": id,
	}

	_, err := d.request("https://api.febbox.com/oauth", http.MethodPost, func(req *resty.Request) {
		req.SetMultipartFormData(queryParams)
	}, nil)
	if err != nil {
		return err
	}

	return nil
}
