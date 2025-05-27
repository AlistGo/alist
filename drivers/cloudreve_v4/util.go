package cloudreve_v4

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
)

// do others that not defined in Driver interface

func (d *CloudreveV4) getUA() string {
	if d.CustomUA != "" {
		return d.CustomUA
	}
	return base.UserAgent
}

func (d *CloudreveV4) request(method string, path string, callback base.ReqCallback, out any) error {
	if d.ref != nil {
		return d.ref.request(method, path, callback, out)
	}
	u := d.Address + "/api/v4" + path
	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		"Accept":     "application/json, text/plain, */*",
		"User-Agent": d.getUA(),
	})
	if d.AccessToken != "" {
		req.SetHeader("Authorization", "Bearer "+d.AccessToken)
	}

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
		if r.Code == 401 && d.RefreshToken != "" && path != "/session/token/refresh" {
			// try to refresh token
			err = d.refreshToken()
			if err != nil {
				return err
			}
			return d.request(method, path, callback, out)
		}
		return errors.New(r.Msg)
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

func (d *CloudreveV4) login() error {
	var siteConfig SiteLoginConfigResp
	err := d.request(http.MethodGet, "/site/config/login", nil, &siteConfig)
	if err != nil {
		return err
	}
	if !siteConfig.Authn {
		return errors.New("authn not support")
	}
	var prepareLogin PrepareLoginResp
	err = d.request(http.MethodGet, "/session/prepare?email="+d.Addition.Username, nil, &prepareLogin)
	if err != nil {
		return err
	}
	if !prepareLogin.PasswordEnabled {
		return errors.New("password not enabled")
	}
	if prepareLogin.WebauthnEnabled {
		return errors.New("webauthn not support")
	}
	for range 5 {
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

func (d *CloudreveV4) doLogin(needCaptcha bool) error {
	var err error
	loginBody := base.Json{
		"email":    d.Username,
		"password": d.Password,
	}
	if needCaptcha {
		var config BasicConfigResp
		err = d.request(http.MethodGet, "/site/config/basic", nil, &config)
		if err != nil {
			return err
		}
		if config.CaptchaType != "normal" {
			return fmt.Errorf("captcha type %s not support", config.CaptchaType)
		}
		var captcha CaptchaResp
		err = d.request(http.MethodGet, "/site/captcha", nil, &captcha)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(captcha.Image, "data:image/png;base64,") {
			return errors.New("can not get captcha")
		}
		loginBody["ticket"] = captcha.Ticket
		i := strings.Index(captcha.Image, ",")
		dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(captcha.Image[i+1:]))
		vRes, err := base.RestyClient.R().SetMultipartField(
			"image", "validateCode.png", "image/png", dec).
			Post(setting.GetStr(conf.OcrApi))
		if err != nil {
			return err
		}
		if jsoniter.Get(vRes.Body(), "status").ToInt() != 200 {
			return errors.New("ocr error:" + jsoniter.Get(vRes.Body(), "msg").ToString())
		}
		captchaCode := jsoniter.Get(vRes.Body(), "result").ToString()
		if captchaCode == "" {
			return errors.New("ocr error: empty result")
		}
		loginBody["captcha"] = captchaCode
	}
	var token TokenResponse
	err = d.request(http.MethodPost, "/session/token", func(req *resty.Request) {
		req.SetBody(loginBody)
	}, &token)
	if err != nil {
		return err
	}
	d.AccessToken, d.RefreshToken = token.Token.AccessToken, token.Token.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *CloudreveV4) refreshToken() error {
	var token Token
	if token.RefreshToken == "" {
		if d.Username != "" {
			err := d.login()
			if err != nil {
				return fmt.Errorf("cannot login to get refresh token, error: %s", err)
			}
		}
		return nil
	}
	err := d.request(http.MethodPost, "/session/token/refresh", func(req *resty.Request) {
		req.SetBody(base.Json{
			"refresh_token": d.RefreshToken,
		})
	}, &token)
	if err != nil {
		return err
	}
	d.AccessToken, d.RefreshToken = token.AccessToken, token.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *CloudreveV4) upLocal(ctx context.Context, file model.FileStreamer, u FileUploadResp, up driver.UpdateProgress) error {
	var finish int64 = 0
	var chunk int = 0
	DEFAULT := int64(u.ChunkSize)
	if DEFAULT == 0 {
		// support relay
		DEFAULT = file.GetSize()
	}
	for finish < file.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		left := file.GetSize() - finish
		byteSize := min(left, DEFAULT)
		utils.Log.Debugf("[CloudreveV4-Local] upload range: %d-%d/%d", finish, finish+byteSize-1, file.GetSize())
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(file, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		err = d.request(http.MethodPost, "/file/upload/"+u.SessionID+"/"+strconv.Itoa(chunk), func(req *resty.Request) {
			req.SetHeader("Content-Type", "application/octet-stream")
			req.SetContentLength(true)
			req.SetHeader("Content-Length", strconv.FormatInt(byteSize, 10))
			req.SetBody(driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
			req.AddRetryCondition(func(r *resty.Response, err error) bool {
				if err != nil {
					return true
				}
				if r.IsError() {
					return true
				}
				var retryResp Resp
				jErr := base.RestyClient.JSONUnmarshal(r.Body(), &retryResp)
				if jErr != nil {
					return true
				}
				if retryResp.Code != 0 {
					return true
				}
				return false
			})
		}, nil)
		if err != nil {
			return err
		}
		finish += byteSize
		up(float64(finish) * 100 / float64(file.GetSize()))
		chunk++
	}
	return nil
}

func (d *CloudreveV4) upRemote(ctx context.Context, file model.FileStreamer, u FileUploadResp, up driver.UpdateProgress) error {
	uploadUrl := u.UploadUrls[0]
	credential := u.Credential
	var finish int64 = 0
	var chunk int = 0
	DEFAULT := int64(u.ChunkSize)
	retryCount := 0
	maxRetries := 3
	for finish < file.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		left := file.GetSize() - finish
		byteSize := min(left, DEFAULT)
		utils.Log.Debugf("[CloudreveV4-Remote] upload range: %d-%d/%d", finish, finish+byteSize-1, file.GetSize())
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(file, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("POST", uploadUrl+"?chunk="+strconv.Itoa(chunk),
			driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		// req.Header.Set("Content-Length", strconv.Itoa(int(byteSize)))
		req.Header.Set("Authorization", fmt.Sprint(credential))
		req.Header.Set("User-Agent", d.getUA())
		err = func() error {
			res, err := base.HttpClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return errors.New(res.Status)
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}
			var up Resp
			err = json.Unmarshal(body, &up)
			if err != nil {
				return err
			}
			if up.Code != 0 {
				return errors.New(up.Msg)
			}
			return nil
		}()
		if err == nil {
			retryCount = 0
			finish += byteSize
			up(float64(finish) * 100 / float64(file.GetSize()))
			chunk++
		} else {
			retryCount++
			if retryCount > maxRetries {
				return fmt.Errorf("upload failed after %d retries due to server errors, error: %s", maxRetries, err)
			}
			backoff := time.Duration(1<<retryCount) * time.Second
			utils.Log.Warnf("[Cloudreve-Remote] server errors while uploading, retrying after %v...", backoff)
			time.Sleep(backoff)
		}
	}
	return nil
}

func (d *CloudreveV4) upOneDrive(ctx context.Context, file model.FileStreamer, u FileUploadResp, up driver.UpdateProgress) error {
	uploadUrl := u.UploadUrls[0]
	var finish int64 = 0
	DEFAULT := int64(u.ChunkSize)
	retryCount := 0
	maxRetries := 3
	for finish < file.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		left := file.GetSize() - finish
		byteSize := min(left, DEFAULT)
		utils.Log.Debugf("[CloudreveV4-OneDrive] upload range: %d-%d/%d", finish, finish+byteSize-1, file.GetSize())
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(file, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodPut, uploadUrl, driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		// req.Header.Set("Content-Length", strconv.Itoa(int(byteSize)))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", finish, finish+byteSize-1, file.GetSize()))
		req.Header.Set("User-Agent", d.getUA())
		finish += byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		// https://learn.microsoft.com/zh-cn/onedrive/developer/rest-api/api/driveitem_createuploadsession
		switch {
		case res.StatusCode >= 500 && res.StatusCode <= 504:
			retryCount++
			if retryCount > maxRetries {
				res.Body.Close()
				return fmt.Errorf("upload failed after %d retries due to server errors, error %d", maxRetries, res.StatusCode)
			}
			backoff := time.Duration(1<<retryCount) * time.Second
			utils.Log.Warnf("[CloudreveV4-OneDrive] server errors %d while uploading, retrying after %v...", res.StatusCode, backoff)
			time.Sleep(backoff)
		case res.StatusCode != 201 && res.StatusCode != 202 && res.StatusCode != 200:
			data, _ := io.ReadAll(res.Body)
			res.Body.Close()
			return errors.New(string(data))
		default:
			res.Body.Close()
			retryCount = 0
			finish += byteSize
			up(float64(finish) * 100 / float64(file.GetSize()))
		}
	}
	// 上传成功发送回调请求
	return d.request(http.MethodPost, "/callback/onedrive/"+u.SessionID+"/"+u.CallbackSecret, func(req *resty.Request) {
		req.SetBody("{}")
	}, nil)
}

func (d *CloudreveV4) upS3(ctx context.Context, file model.FileStreamer, u FileUploadResp, up driver.UpdateProgress) error {
	var finish int64 = 0
	var chunk int = 0
	var etags []string
	DEFAULT := int64(u.ChunkSize)
	retryCount := 0
	maxRetries := 3
	for finish < file.GetSize() {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		left := file.GetSize() - finish
		byteSize := min(left, DEFAULT)
		utils.Log.Debugf("[CloudreveV4-S3] upload range: %d-%d/%d", finish, finish+byteSize-1, file.GetSize())
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(file, byteData)
		utils.Log.Debug(err, n)
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodPut, u.UploadUrls[chunk],
			driver.NewLimitedUploadStream(ctx, bytes.NewBuffer(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		req.ContentLength = byteSize
		res, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		etag := res.Header.Get("ETag")
		res.Body.Close()
		switch {
		case res.StatusCode != 200:
			retryCount++
			if retryCount > maxRetries {
				return fmt.Errorf("upload failed after %d retries due to server errors", maxRetries)
			}
			backoff := time.Duration(1<<retryCount) * time.Second
			utils.Log.Warnf("server error %d, retrying after %v...", res.StatusCode, backoff)
			time.Sleep(backoff)
		case etag == "":
			return errors.New("faild to get ETag from header")
		default:
			retryCount = 0
			etags = append(etags, etag)
			finish += byteSize
			up(float64(finish) * 100 / float64(file.GetSize()))
			chunk++
		}
	}

	// s3LikeFinishUpload
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
	return d.request(http.MethodPost, "/callback/s3/"+u.SessionID+"/"+u.CallbackSecret, func(req *resty.Request) {
		req.SetBody("{}")
	}, nil)
}
