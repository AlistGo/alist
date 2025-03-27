package cloudreve

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/cookie"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	json "github.com/json-iterator/go"
	jsoniter "github.com/json-iterator/go"
)

// do others that not defined in Driver interface

const loginPath = "/user/session"

func (d *Cloudreve) getUA() string {
	if d.CustomUA != "" {
		return d.CustomUA
	}
	return base.UserAgent
}

func (d *Cloudreve) request(method string, path string, callback base.ReqCallback, out interface{}) error {
	u := d.Address + "/api/v3" + path
	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		"Cookie":     "cloudreve-session=" + d.Cookie,
		"Accept":     "application/json, text/plain, */*",
		"User-Agent": d.getUA(),
	})

	var r Resp
	req.SetResult(&r)

	if callback != nil {
		callback(req)
	}

	resp, err := req.Execute(method, u)
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return errors.New(resp.String())
	}

	if r.Code != 0 {

		// 刷新 cookie
		if r.Code == http.StatusUnauthorized && path != loginPath {
			if d.Username != "" && d.Password != "" {
				err = d.login()
				if err != nil {
					return err
				}
				return d.request(method, path, callback, out)
			}
		}

		return errors.New(r.Msg)
	}
	sess := cookie.GetCookie(resp.Cookies(), "cloudreve-session")
	if sess != nil {
		d.Cookie = sess.Value
	}
	if out != nil && r.Data != nil {
		var marshal []byte
		marshal, err = json.Marshal(r.Data)
		if err != nil {
			return err
		}
		err = json.Unmarshal(marshal, out)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Cloudreve) login() error {
	var siteConfig Config
	err := d.request(http.MethodGet, "/site/config", nil, &siteConfig)
	if err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		err = d.doLogin(siteConfig.LoginCaptcha)
		if err == nil {
			break
		}
		if err.Error() != "CAPTCHA not match." {
			break
		}
	}
	return err
}

func (d *Cloudreve) doLogin(needCaptcha bool) error {
	var captchaCode string
	var err error
	if needCaptcha {
		var captcha string
		err = d.request(http.MethodGet, "/site/captcha", nil, &captcha)
		if err != nil {
			return err
		}
		if len(captcha) == 0 {
			return errors.New("can not get captcha")
		}
		i := strings.Index(captcha, ",")
		dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(captcha[i+1:]))
		vRes, err := base.RestyClient.R().SetMultipartField(
			"image", "validateCode.png", "image/png", dec).
			Post(setting.GetStr(conf.OcrApi))
		if err != nil {
			return err
		}
		if jsoniter.Get(vRes.Body(), "status").ToInt() != 200 {
			return errors.New("ocr error:" + jsoniter.Get(vRes.Body(), "msg").ToString())
		}
		captchaCode = jsoniter.Get(vRes.Body(), "result").ToString()
	}
	var resp Resp
	err = d.request(http.MethodPost, loginPath, func(req *resty.Request) {
		req.SetBody(base.Json{
			"username":    d.Addition.Username,
			"Password":    d.Addition.Password,
			"captchaCode": captchaCode,
		})
	}, &resp)
	return err
}

func convertSrc(obj model.Obj) map[string]interface{} {
	m := make(map[string]interface{})
	var dirs []string
	var items []string
	if obj.IsDir() {
		dirs = append(dirs, obj.GetID())
	} else {
		items = append(items, obj.GetID())
	}
	m["dirs"] = dirs
	m["items"] = items
	return m
}

func (d *Cloudreve) GetThumb(file Object) (model.Thumbnail, error) {
	if !d.Addition.EnableThumbAndFolderSize {
		return model.Thumbnail{}, nil
	}
	req := base.NoRedirectClient.R()
	req.SetHeaders(map[string]string{
		"Cookie":     "cloudreve-session=" + d.Cookie,
		"Accept":     "image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"User-Agent": d.getUA(),
	})
	resp, err := req.Execute(http.MethodGet, d.Address+"/api/v3/file/thumb/"+file.Id)
	if err != nil {
		return model.Thumbnail{}, err
	}
	return model.Thumbnail{
		Thumbnail: resp.Header().Get("Location"),
	}, nil
}

func (d *Cloudreve) upLocal(ctx context.Context, stream model.FileStreamer, u UploadInfo, up driver.UpdateProgress) error {
	var finish int64 = 0
	var chunk int = 0
	DEFAULT := int64(u.ChunkSize)
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		utils.Log.Debugf("[Cloudreve-Local] upload: %d", finish)
		var byteSize = DEFAULT
		left := stream.GetSize() - finish
		if left < DEFAULT {
			byteSize = left
		}
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		err = d.request(http.MethodPost, "/file/upload/"+u.SessionID+"/"+strconv.Itoa(chunk), func(req *resty.Request) {
			req.SetHeader("Content-Type", "application/octet-stream")
			req.SetContentLength(true)
			req.SetHeader("Content-Length", strconv.FormatInt(byteSize, 10))
			req.SetHeader("User-Agent", d.getUA())
			req.SetBody(driver.NewLimitedUploadStream(ctx, bytes.NewBuffer(byteData)))
		}, nil)
		if err != nil {
			break
		}
		finish += byteSize
		up(float64(finish) * 100 / float64(stream.GetSize()))
		chunk++
	}
	return nil
}

func (d *Cloudreve) upRemote(ctx context.Context, stream model.FileStreamer, u UploadInfo, up driver.UpdateProgress) error {
	uploadUrl := u.UploadURLs[0]
	credential := u.Credential
	var finish int64 = 0
	var chunk int = 0
	DEFAULT := int64(u.ChunkSize)
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		utils.Log.Debugf("[Cloudreve-Remote] upload: %d", finish)
		var byteSize = DEFAULT
		left := stream.GetSize() - finish
		if left < DEFAULT {
			byteSize = left
		}
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("POST", uploadUrl+"?chunk="+strconv.Itoa(chunk),
			driver.NewLimitedUploadStream(ctx, bytes.NewBuffer(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		// req.Header.Set("Content-Length", strconv.Itoa(int(byteSize)))
		req.Header.Set("Authorization", fmt.Sprint(credential))
		req.Header.Set("User-Agent", d.getUA())
		finish += byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		_ = res.Body.Close()
		up(float64(finish) * 100 / float64(stream.GetSize()))
		chunk++
	}
	return nil
}

func (d *Cloudreve) upOneDrive(ctx context.Context, stream model.FileStreamer, u UploadInfo, up driver.UpdateProgress) error {
	uploadUrl := u.UploadURLs[0]
	var finish int64 = 0
	DEFAULT := int64(u.ChunkSize)
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		utils.Log.Debugf("[Cloudreve-OneDrive] upload: %d", finish)
		var byteSize = DEFAULT
		left := stream.GetSize() - finish
		if left < DEFAULT {
			byteSize = left
		}
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("PUT", uploadUrl, driver.NewLimitedUploadStream(ctx, bytes.NewBuffer(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		// req.Header.Set("Content-Length", strconv.Itoa(int(byteSize)))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", finish, finish+byteSize-1, stream.GetSize()))
		req.Header.Set("User-Agent", d.getUA())
		finish += byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		// https://learn.microsoft.com/zh-cn/onedrive/developer/rest-api/api/driveitem_createuploadsession
		if res.StatusCode != 201 && res.StatusCode != 202 && res.StatusCode != 200 {
			data, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			return errors.New(string(data))
		}
		_ = res.Body.Close()
		up(float64(finish) * 100 / float64(stream.GetSize()))
	}
	// 上传成功发送回调请求
	err := d.request(http.MethodPost, "/callback/onedrive/finish/"+u.SessionID, func(req *resty.Request) {
		req.SetBody("{}")
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

func (d *Cloudreve) upS3(ctx context.Context, stream model.FileStreamer, u UploadInfo, up driver.UpdateProgress) error {
	var finish int64 = 0
	var chunk int = 0
	var etags []string
	DEFAULT := int64(u.ChunkSize)
	for finish < stream.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		utils.Log.Debugf("[Cloudreve-S3] upload: %d", finish)
		var byteSize = DEFAULT
		left := stream.GetSize() - finish
		if left < DEFAULT {
			byteSize = left
		}
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(stream, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("PUT", u.UploadURLs[chunk],
			driver.NewLimitedUploadStream(ctx, bytes.NewBuffer(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		finish += byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		_ = res.Body.Close()
		etags = append(etags, res.Header.Get("ETag"))
		up(float64(finish) * 100 / float64(stream.GetSize()))
		chunk++
	}

	// s3LikeFinishUpload
	// https://github.com/cloudreve/frontend/blob/b485bf297974cbe4834d2e8e744ae7b7e5b2ad39/src/component/Uploader/core/api/index.ts#L204-L252
	bodyBuilder := &strings.Builder{}
	bodyBuilder.WriteString("<CompleteMultipartUpload>")
	for i, etag := range etags {
		bodyBuilder.WriteString(fmt.Sprintf(
			`<Part><PartNumber>%d</PartNumber><ETag>%s</ETag></Part>`,
			i+1, // PartNumber 从 1 开始
			etag,
		))
	}
	bodyBuilder.WriteString("</CompleteMultipartUpload>")
	req, err := http.NewRequest(
		"POST",
		u.CompleteURL,
		strings.NewReader(bodyBuilder.String()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("User-Agent", d.getUA())
	res, err := base.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("up status: %d, error: %s", res.StatusCode, string(body))
	}

	// 上传成功发送回调请求
	err = d.request(http.MethodGet, "/callback/s3/"+u.SessionID, nil, nil)
	if err != nil {
		return err
	}
	return nil
}
