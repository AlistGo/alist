package pikpak

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

// do others that not defined in Driver interface

var AndroidAlgorithms = []string{
	"aDhgaSE3MsjROCmpmsWqP1sJdFJ",
	"+oaVkqdd8MJuKT+uMr2AYKcd9tdWge3XPEPR2hcePUknd",
	"u/sd2GgT2fTytRcKzGicHodhvIltMntA3xKw2SRv7S48OdnaQIS5mn",
	"2WZiae2QuqTOxBKaaqCNHCW3olu2UImelkDzBn",
	"/vJ3upic39lgmrkX855Qx",
	"yNc9ruCVMV7pGV7XvFeuLMOcy1",
	"4FPq8mT3JQ1jzcVxMVfwFftLQm33M7i",
	"xozoy5e3Ea",
}

var WebAlgorithms = []string{
	"C9qPpZLN8ucRTaTiUMWYS9cQvWOE",
	"+r6CQVxjzJV6LCV",
	"F",
	"pFJRC",
	"9WXYIDGrwTCz2OiVlgZa90qpECPD6olt",
	"/750aCr4lm/Sly/c",
	"RB+DT/gZCrbV",
	"",
	"CyLsf7hdkIRxRm215hl",
	"7xHvLi2tOYP0Y92b",
	"ZGTXXxu8E/MIWaEDB+Sm/",
	"1UI3",
	"E7fP5Pfijd+7K+t6Tg/NhuLq0eEUVChpJSkrKxpO",
	"ihtqpG6FMt65+Xk+tWUH2",
	"NhXXU9rg4XXdzo7u5o",
}

var PCAlgorithms = []string{
	"KHBJ07an7ROXDoK7Db",
	"G6n399rSWkl7WcQmw5rpQInurc1DkLmLJqE",
	"JZD1A3M4x+jBFN62hkr7VDhkkZxb9g3rWqRZqFAAb",
	"fQnw/AmSlbbI91Ik15gpddGgyU7U",
	"/Dv9JdPYSj3sHiWjouR95NTQff",
	"yGx2zuTjbWENZqecNI+edrQgqmZKP",
	"ljrbSzdHLwbqcRn",
	"lSHAsqCkGDGxQqqwrVu",
	"TsWXI81fD1",
	"vk7hBjawK/rOSrSWajtbMk95nfgf3",
}

const (
	OSSUserAgent               = "aliyun-sdk-android/2.9.13(Linux/Android 14/M2004j7ac;UKQ1.231108.001)"
	OssSecurityTokenHeaderName = "X-OSS-Security-Token"
	ThreadsNum                 = 10
)

const (
	AndroidClientID      = "YNxT9w7GMdWvEOKa"
	AndroidClientSecret  = "dbw2OtmVEeuUvIptb1Coyg"
	AndroidClientVersion = "1.48.3"
	AndroidPackageName   = "com.pikcloud.pikpak"
	AndroidSdkVersion    = "2.0.4.204101"
	WebClientID          = "YUMx5nI8ZU8Ap8pm"
	WebClientSecret      = "dbw2OtmVEeuUvIptb1Coyg"
	WebClientVersion     = "2.0.0"
	WebPackageName       = "mypikpak.net"
	WebSdkVersion        = "8.0.3"
	PCClientID           = "YvtoWO6GNHiuCl7x"
	PCClientSecret       = "1NIH5R1IEe2pAxZE3hv3uA"
	PCClientVersion      = "undefined" // 2.5.6.4831
	PCPackageName        = "mypikpak.net"
	PCSdkVersion         = "8.0.3"
)

var DlAddr = []string{
	"dl-a10b-0621.mypikpak.net",
	"dl-a10b-0622.mypikpak.net",
	"dl-a10b-0623.mypikpak.net",
	"dl-a10b-0624.mypikpak.net",
	"dl-a10b-0625.mypikpak.net",
	"dl-a10b-0858.mypikpak.net",
	"dl-a10b-0859.mypikpak.net",
	"dl-a10b-0860.mypikpak.net",
	"dl-a10b-0861.mypikpak.net",
	"dl-a10b-0862.mypikpak.net",
	"dl-a10b-0863.mypikpak.net",
	"dl-a10b-0864.mypikpak.net",
	"dl-a10b-0865.mypikpak.net",
	"dl-a10b-0866.mypikpak.net",
	"dl-a10b-0867.mypikpak.net",
	"dl-a10b-0868.mypikpak.net",
	"dl-a10b-0869.mypikpak.net",
	"dl-a10b-0870.mypikpak.net",
	"dl-a10b-0871.mypikpak.net",
	"dl-a10b-0872.mypikpak.net",
	"dl-a10b-0873.mypikpak.net",
	"dl-a10b-0874.mypikpak.net",
	"dl-a10b-0875.mypikpak.net",
	"dl-a10b-0876.mypikpak.net",
	"dl-a10b-0877.mypikpak.net",
	"dl-a10b-0878.mypikpak.net",
	"dl-a10b-0879.mypikpak.net",
	"dl-a10b-0880.mypikpak.net",
	"dl-a10b-0881.mypikpak.net",
	"dl-a10b-0882.mypikpak.net",
	"dl-a10b-0883.mypikpak.net",
	"dl-a10b-0884.mypikpak.net",
	"dl-a10b-0885.mypikpak.net",
	"dl-a10b-0886.mypikpak.net",
	"dl-a10b-0887.mypikpak.net",
}

func (d *PikPak) login() error {
	// 检查用户名和密码是否为空
	if d.Addition.Username == "" || d.Addition.Password == "" {
		return errors.New("username or password is empty")
	}

	url := "https://user.mypikpak.net/v1/auth/signin"
	// 使用 用户填写的 CaptchaToken —————— (验证后的captcha_token)
	if d.GetCaptchaToken() == "" {
		if err := d.RefreshCaptchaTokenInLogin(GetAction(http.MethodPost, url), d.Username); err != nil {
			return err
		}
	}

	var e ErrResp
	res, err := base.RestyClient.SetRetryCount(1).R().SetError(&e).SetBody(base.Json{
		"captcha_token": d.GetCaptchaToken(),
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
		"username":      d.Username,
		"password":      d.Password,
	}).SetQueryParam("client_id", d.ClientID).Post(url)
	if err != nil {
		return err
	}
	if e.ErrorCode != 0 {
		return &e
	}
	data := res.Body()
	d.RefreshToken = jsoniter.Get(data, "refresh_token").ToString()
	d.AccessToken = jsoniter.Get(data, "access_token").ToString()
	d.Common.SetUserID(jsoniter.Get(data, "sub").ToString())
	return nil
}

func (d *PikPak) refreshToken(refreshToken string) error {
	url := "https://user.mypikpak.net/v1/auth/token"
	var e ErrResp
	res, err := base.RestyClient.SetRetryCount(1).R().SetError(&e).
		SetHeader("user-agent", "").SetBody(base.Json{
		"client_id":     d.ClientID,
		"client_secret": d.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
	}).SetQueryParam("client_id", d.ClientID).Post(url)
	if err != nil {
		d.Status = err.Error()
		op.MustSaveDriverStorage(d)
		return err
	}
	if e.ErrorCode != 0 {
		if e.ErrorCode == 4126 {
			// 1. 未填写 username 或 password
			if d.Addition.Username == "" || d.Addition.Password == "" {
				return errors.New("refresh_token invalid, please re-provide refresh_token")
			} else {
				// refresh_token invalid, re-login
				return d.login()
			}
		}
		d.Status = e.Error()
		op.MustSaveDriverStorage(d)
		return errors.New(e.Error())
	}
	data := res.Body()
	d.Status = "work"
	d.RefreshToken = jsoniter.Get(data, "refresh_token").ToString()
	d.AccessToken = jsoniter.Get(data, "access_token").ToString()
	d.Common.SetUserID(jsoniter.Get(data, "sub").ToString())
	d.Addition.RefreshToken = d.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *PikPak) initializeOAuth2Token(ctx context.Context, oauth2Config *oauth2.Config, refreshToken string) {
	d.oauth2Token = oauth2.ReuseTokenSource(nil, utils.TokenSource(func() (*oauth2.Token, error) {
		return oauth2Config.TokenSource(ctx, &oauth2.Token{
			RefreshToken: refreshToken,
		}).Token()
	}))
}

func (d *PikPak) refreshTokenByOAuth2() error {
	token, err := d.oauth2Token.Token()
	if err != nil {
		return err
	}
	d.Status = "work"
	d.RefreshToken = token.RefreshToken
	d.AccessToken = token.AccessToken
	// 获取用户ID
	userID := token.Extra("sub").(string)
	d.Common.SetUserID(userID)
	d.Addition.RefreshToken = d.RefreshToken
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *PikPak) request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		//"Authorization":   "Bearer " + d.AccessToken,
		"User-Agent":      d.GetUserAgent(),
		"X-Device-ID":     d.GetDeviceID(),
		"X-Captcha-Token": d.GetCaptchaToken(),
	})
	if d.RefreshTokenMethod == "oauth2" && d.oauth2Token != nil {
		// 使用oauth2 获取 access_token
		token, err := d.oauth2Token.Token()
		if err != nil {
			return nil, err
		}
		req.SetAuthScheme(token.TokenType).SetAuthToken(token.AccessToken)
	} else if d.AccessToken != "" {
		req.SetHeader("Authorization", "Bearer "+d.AccessToken)
	}

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
	case 4122, 4121, 16:
		// access_token 过期
		if d.RefreshTokenMethod == "oauth2" {
			if err1 := d.refreshTokenByOAuth2(); err1 != nil {
				return nil, err1
			}
		} else {
			if err1 := d.refreshToken(d.RefreshToken); err1 != nil {
				return nil, err1
			}
		}

		return d.request(url, method, callback, resp)
	case 9: // 验证码token过期
		if err = d.RefreshCaptchaTokenAtLogin(GetAction(method, url), d.GetUserID()); err != nil {
			return nil, err
		}
		return d.request(url, method, callback, resp)
	case 10: // 操作频繁
		return nil, errors.New(e.ErrorDescription)
	default:
		return nil, errors.New(e.Error())
	}
}

func (d *PikPak) getFiles(id string) ([]File, error) {
	res := make([]File, 0)
	pageToken := "first"
	for pageToken != "" {
		if pageToken == "first" {
			pageToken = ""
		}
		query := map[string]string{
			"parent_id":      id,
			"thumbnail_size": "SIZE_LARGE",
			"with_audit":     "true",
			"limit":          "100",
			"filters":        `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
			"page_token":     pageToken,
		}
		var resp Files
		_, err := d.request("https://api-drive.mypikpak.net/drive/v1/files", http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(query)
		}, &resp)
		if err != nil {
			return nil, err
		}
		pageToken = resp.NextPageToken
		res = append(res, resp.Files...)
	}
	return res, nil
}

func GetAction(method string, url string) string {
	urlpath := regexp.MustCompile(`://[^/]+((/[^/\s?#]+)*)`).FindStringSubmatch(url)[1]
	return method + ":" + urlpath
}

type Common struct {
	client       *resty.Client
	CaptchaToken string
	UserID       string
	// 必要值,签名相关
	ClientID      string
	ClientSecret  string
	ClientVersion string
	PackageName   string
	Algorithms    []string
	DeviceID      string
	UserAgent     string
	// 验证码token刷新成功回调
	RefreshCTokenCk func(token string)
	LowLatencyAddr  string
}

func generateDeviceSign(deviceID, packageName string) string {

	signatureBase := fmt.Sprintf("%s%s%s%s", deviceID, packageName, "1", "appkey")

	sha1Hash := sha1.New()
	sha1Hash.Write([]byte(signatureBase))
	sha1Result := sha1Hash.Sum(nil)

	sha1String := hex.EncodeToString(sha1Result)

	md5Hash := md5.New()
	md5Hash.Write([]byte(sha1String))
	md5Result := md5Hash.Sum(nil)

	md5String := hex.EncodeToString(md5Result)

	deviceSign := fmt.Sprintf("div101.%s%s", deviceID, md5String)

	return deviceSign
}

func BuildCustomUserAgent(deviceID, clientID, appName, sdkVersion, clientVersion, packageName, userID string) string {
	deviceSign := generateDeviceSign(deviceID, packageName)
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("ANDROID-%s/%s ", appName, clientVersion))
	sb.WriteString("protocolVersion/200 ")
	sb.WriteString("accesstype/ ")
	sb.WriteString(fmt.Sprintf("clientid/%s ", clientID))
	sb.WriteString(fmt.Sprintf("clientversion/%s ", clientVersion))
	sb.WriteString("action_type/ ")
	sb.WriteString("networktype/WIFI ")
	sb.WriteString("sessionid/ ")
	sb.WriteString(fmt.Sprintf("deviceid/%s ", deviceID))
	sb.WriteString("providername/NONE ")
	sb.WriteString(fmt.Sprintf("devicesign/%s ", deviceSign))
	sb.WriteString("refresh_token/ ")
	sb.WriteString(fmt.Sprintf("sdkversion/%s ", sdkVersion))
	sb.WriteString(fmt.Sprintf("datetime/%d ", time.Now().UnixMilli()))
	sb.WriteString(fmt.Sprintf("usrno/%s ", userID))
	sb.WriteString(fmt.Sprintf("appname/android-%s ", appName))
	sb.WriteString(fmt.Sprintf("session_origin/ "))
	sb.WriteString(fmt.Sprintf("grant_type/ "))
	sb.WriteString(fmt.Sprintf("appid/ "))
	sb.WriteString(fmt.Sprintf("clientip/ "))
	sb.WriteString(fmt.Sprintf("devicename/Xiaomi_M2004j7ac "))
	sb.WriteString(fmt.Sprintf("osversion/13 "))
	sb.WriteString(fmt.Sprintf("platformversion/10 "))
	sb.WriteString(fmt.Sprintf("accessmode/ "))
	sb.WriteString(fmt.Sprintf("devicemodel/M2004J7AC "))

	return sb.String()
}

func (c *Common) SetDeviceID(deviceID string) {
	c.DeviceID = deviceID
}

func (c *Common) SetUserID(userID string) {
	c.UserID = userID
}

func (c *Common) SetUserAgent(userAgent string) {
	c.UserAgent = userAgent
}

func (c *Common) SetCaptchaToken(captchaToken string) {
	c.CaptchaToken = captchaToken
}
func (c *Common) GetCaptchaToken() string {
	return c.CaptchaToken
}

func (c *Common) GetUserAgent() string {
	return c.UserAgent
}

func (c *Common) GetDeviceID() string {
	return c.DeviceID
}

func (c *Common) GetUserID() string {
	return c.UserID
}

// RefreshCaptchaTokenAtLogin 刷新验证码token(登录后)
func (d *PikPak) RefreshCaptchaTokenAtLogin(action, userID string) error {
	metas := map[string]string{
		"client_version": d.ClientVersion,
		"package_name":   d.PackageName,
		"user_id":        userID,
	}
	metas["timestamp"], metas["captcha_sign"] = d.Common.GetCaptchaSign()
	return d.refreshCaptchaToken(action, metas)
}

// RefreshCaptchaTokenInLogin 刷新验证码token(登录时)
func (d *PikPak) RefreshCaptchaTokenInLogin(action, username string) error {
	metas := make(map[string]string)
	if ok, _ := regexp.MatchString(`\w+([-+.]\w+)*@\w+([-.]\w+)*\.\w+([-.]\w+)*`, username); ok {
		metas["email"] = username
	} else if len(username) >= 11 && len(username) <= 18 {
		metas["phone_number"] = username
	} else {
		metas["username"] = username
	}
	return d.refreshCaptchaToken(action, metas)
}

// GetCaptchaSign 获取验证码签名
func (c *Common) GetCaptchaSign() (timestamp, sign string) {
	timestamp = fmt.Sprint(time.Now().UnixMilli())
	str := fmt.Sprint(c.ClientID, c.ClientVersion, c.PackageName, c.DeviceID, timestamp)
	for _, algorithm := range c.Algorithms {
		str = utils.GetMD5EncodeStr(str + algorithm)
	}
	sign = "1." + str
	return
}

// refreshCaptchaToken 刷新CaptchaToken
func (d *PikPak) refreshCaptchaToken(action string, metas map[string]string) error {
	param := CaptchaTokenRequest{
		Action:       action,
		CaptchaToken: d.GetCaptchaToken(),
		ClientID:     d.ClientID,
		DeviceID:     d.GetDeviceID(),
		Meta:         metas,
		RedirectUri:  "xlaccsdk01://xbase.cloud/callback?state=harbor",
	}
	var e ErrResp
	var resp CaptchaTokenResponse
	_, err := d.request("https://user.mypikpak.net/v1/shield/captcha/init", http.MethodPost, func(req *resty.Request) {
		req.SetError(&e).SetBody(param).SetQueryParam("client_id", d.ClientID)
	}, &resp)

	if err != nil {
		return err
	}

	if e.IsError() {
		return errors.New(e.Error())
	}

	if resp.Url != "" {
		return fmt.Errorf(`need verify: <a target="_blank" href="%s">Click Here</a>`, resp.Url)
	}

	if d.Common.RefreshCTokenCk != nil {
		d.Common.RefreshCTokenCk(resp.CaptchaToken)
	}
	d.Common.SetCaptchaToken(resp.CaptchaToken)
	return nil
}

func (d *PikPak) UploadByOSS(params *S3Params, stream model.FileStreamer, up driver.UpdateProgress) error {
	ossClient, err := oss.New(params.Endpoint, params.AccessKeyID, params.AccessKeySecret)
	if err != nil {
		return err
	}
	bucket, err := ossClient.Bucket(params.Bucket)
	if err != nil {
		return err
	}

	err = bucket.PutObject(params.Key, stream, OssOption(params)...)
	if err != nil {
		return err
	}
	return nil
}

func (d *PikPak) UploadByMultipart(params *S3Params, fileSize int64, stream model.FileStreamer, up driver.UpdateProgress) error {
	var (
		chunks    []oss.FileChunk
		parts     []oss.UploadPart
		imur      oss.InitiateMultipartUploadResult
		ossClient *oss.Client
		bucket    *oss.Bucket
		err       error
	)

	tmpF, err := stream.CacheFullInTempFile()
	if err != nil {
		return err
	}

	if ossClient, err = oss.New(params.Endpoint, params.AccessKeyID, params.AccessKeySecret); err != nil {
		return err
	}

	if bucket, err = ossClient.Bucket(params.Bucket); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Hour * 12)
	defer ticker.Stop()
	// 设置超时
	timeout := time.NewTimer(time.Hour * 24)

	if chunks, err = SplitFile(fileSize); err != nil {
		return err
	}

	if imur, err = bucket.InitiateMultipartUpload(params.Key,
		oss.SetHeader(OssSecurityTokenHeaderName, params.SecurityToken),
		oss.UserAgentHeader(OSSUserAgent),
	); err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(len(chunks))

	chunksCh := make(chan oss.FileChunk)
	errCh := make(chan error)
	UploadedPartsCh := make(chan oss.UploadPart)
	quit := make(chan struct{})

	// producer
	go chunksProducer(chunksCh, chunks)
	go func() {
		wg.Wait()
		quit <- struct{}{}
	}()

	// consumers
	for i := 0; i < ThreadsNum; i++ {
		go func(threadId int) {
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("recovered in %v", r)
				}
			}()
			for chunk := range chunksCh {
				var part oss.UploadPart // 出现错误就继续尝试，共尝试3次
				for retry := 0; retry < 3; retry++ {
					select {
					case <-ticker.C:
						errCh <- errors.Wrap(err, "ossToken 过期")
					default:
					}

					buf := make([]byte, chunk.Size)
					if _, err = tmpF.ReadAt(buf, chunk.Offset); err != nil && !errors.Is(err, io.EOF) {
						continue
					}

					b := bytes.NewBuffer(buf)
					if part, err = bucket.UploadPart(imur, b, chunk.Size, chunk.Number, OssOption(params)...); err == nil {
						break
					}
				}
				if err != nil {
					errCh <- errors.Wrap(err, fmt.Sprintf("上传 %s 的第%d个分片时出现错误：%v", stream.GetName(), chunk.Number, err))
				}
				UploadedPartsCh <- part
			}
		}(i)
	}

	go func() {
		for part := range UploadedPartsCh {
			parts = append(parts, part)
			wg.Done()
		}
	}()
LOOP:
	for {
		select {
		case <-ticker.C:
			// ossToken 过期
			return err
		case <-quit:
			break LOOP
		case <-errCh:
			return err
		case <-timeout.C:
			return fmt.Errorf("time out")
		}
	}

	// EOF错误是xml的Unmarshal导致的，响应其实是json格式，所以实际上上传是成功的
	if _, err = bucket.CompleteMultipartUpload(imur, parts, OssOption(params)...); err != nil && !errors.Is(err, io.EOF) {
		// 当文件名含有 &< 这两个字符之一时响应的xml解析会出现错误，实际上上传是成功的
		if filename := filepath.Base(stream.GetName()); !strings.ContainsAny(filename, "&<") {
			return err
		}
	}
	return nil
}

func chunksProducer(ch chan oss.FileChunk, chunks []oss.FileChunk) {
	for _, chunk := range chunks {
		ch <- chunk
	}
}

func SplitFile(fileSize int64) (chunks []oss.FileChunk, err error) {
	for i := int64(1); i < 10; i++ {
		if fileSize < i*utils.GB { // 文件大小小于iGB时分为i*100片
			if chunks, err = SplitFileByPartNum(fileSize, int(i*100)); err != nil {
				return
			}
			break
		}
	}
	if fileSize > 9*utils.GB { // 文件大小大于9GB时分为1000片
		if chunks, err = SplitFileByPartNum(fileSize, 1000); err != nil {
			return
		}
	}
	// 单个分片大小不能小于1MB
	if chunks[0].Size < 1*utils.MB {
		if chunks, err = SplitFileByPartSize(fileSize, 1*utils.MB); err != nil {
			return
		}
	}
	return
}

// SplitFileByPartNum splits big file into parts by the num of parts.
// Split the file with specified parts count, returns the split result when error is nil.
func SplitFileByPartNum(fileSize int64, chunkNum int) ([]oss.FileChunk, error) {
	if chunkNum <= 0 || chunkNum > 10000 {
		return nil, errors.New("chunkNum invalid")
	}

	if int64(chunkNum) > fileSize {
		return nil, errors.New("oss: chunkNum invalid")
	}

	var chunks []oss.FileChunk
	chunk := oss.FileChunk{}
	chunkN := (int64)(chunkNum)
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * (fileSize / chunkN)
		if i == chunkN-1 {
			chunk.Size = fileSize/chunkN + fileSize%chunkN
		} else {
			chunk.Size = fileSize / chunkN
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// SplitFileByPartSize splits big file into parts by the size of parts.
// Splits the file by the part size. Returns the FileChunk when error is nil.
func SplitFileByPartSize(fileSize int64, chunkSize int64) ([]oss.FileChunk, error) {
	if chunkSize <= 0 {
		return nil, errors.New("chunkSize invalid")
	}

	chunkN := fileSize / chunkSize
	if chunkN >= 10000 {
		return nil, errors.New("Too many parts, please increase part size")
	}

	var chunks []oss.FileChunk
	chunk := oss.FileChunk{}
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * chunkSize
		chunk.Size = chunkSize
		chunks = append(chunks, chunk)
	}

	if fileSize%chunkSize > 0 {
		chunk.Number = len(chunks) + 1
		chunk.Offset = int64(len(chunks)) * chunkSize
		chunk.Size = fileSize % chunkSize
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// OssOption get options
func OssOption(params *S3Params) []oss.Option {
	options := []oss.Option{
		oss.SetHeader(OssSecurityTokenHeaderName, params.SecurityToken),
		oss.UserAgentHeader(OSSUserAgent),
	}
	return options
}

type AddressLatency struct {
	Address string
	Latency time.Duration
}

func checkLatency(address string, wg *sync.WaitGroup, ch chan<- AddressLatency) {
	defer wg.Done()
	start := time.Now()
	resp, err := http.Get("https://" + address + "/generate_204")
	if err != nil {
		ch <- AddressLatency{Address: address, Latency: time.Hour} // Set high latency on error
		return
	}
	defer resp.Body.Close()
	latency := time.Since(start)
	ch <- AddressLatency{Address: address, Latency: latency}
}

func findLowestLatencyAddress(addresses []string) string {
	var wg sync.WaitGroup
	ch := make(chan AddressLatency, len(addresses))

	for _, address := range addresses {
		wg.Add(1)
		go checkLatency(address, &wg, ch)
	}

	wg.Wait()
	close(ch)

	var lowestLatencyAddress string
	lowestLatency := time.Hour

	for result := range ch {
		if result.Latency < lowestLatency {
			lowestLatency = result.Latency
			lowestLatencyAddress = result.Address
		}
	}

	return lowestLatencyAddress
}
