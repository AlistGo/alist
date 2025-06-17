package _189

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	myrand "github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

//func (d *Cloud189) login() error {
//	url := "https://cloud.189.cn/api/portal/loginUrl.action?redirectURL=https%3A%2F%2Fcloud.189.cn%2Fmain.action"
//	b := ""
//	lt := ""
//	ltText := regexp.MustCompile(`lt = "(.+?)"`)
//	var res *resty.Response
//	var err error
//	for i := 0; i < 3; i++ {
//		res, err = d.client.R().Get(url)
//		if err != nil {
//			return err
//		}
//		// 已经登陆
//		if res.RawResponse.Request.URL.String() == "https://cloud.189.cn/web/main" {
//			return nil
//		}
//		b = res.String()
//		ltTextArr := ltText.FindStringSubmatch(b)
//		if len(ltTextArr) > 0 {
//			lt = ltTextArr[1]
//			break
//		} else {
//			<-time.After(time.Second)
//		}
//	}
//	if lt == "" {
//		return fmt.Errorf("get page: %s \nstatus: %d \nrequest url: %s\nredirect url: %s",
//			b, res.StatusCode(), res.RawResponse.Request.URL.String(), res.Header().Get("location"))
//	}
//	captchaToken := regexp.MustCompile(`captchaToken' value='(.+?)'`).FindStringSubmatch(b)[1]
//	returnUrl := regexp.MustCompile(`returnUrl = '(.+?)'`).FindStringSubmatch(b)[1]
//	paramId := regexp.MustCompile(`paramId = "(.+?)"`).FindStringSubmatch(b)[1]
//	//reqId := regexp.MustCompile(`reqId = "(.+?)"`).FindStringSubmatch(b)[1]
//	jRsakey := regexp.MustCompile(`j_rsaKey" value="(\S+)"`).FindStringSubmatch(b)[1]
//	vCodeID := regexp.MustCompile(`picCaptcha\.do\?token\=([A-Za-z0-9\&\=]+)`).FindStringSubmatch(b)[1]
//	vCodeRS := ""
//	if vCodeID != "" {
//		// need ValidateCode
//		log.Debugf("try to identify verification codes")
//		timeStamp := strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
//		u := "https://open.e.189.cn/api/logbox/oauth2/picCaptcha.do?token=" + vCodeID + timeStamp
//		imgRes, err := d.client.R().SetHeaders(map[string]string{
//			"User-Agent":     "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:74.0) Gecko/20100101 Firefox/76.0",
//			"Referer":        "https://open.e.189.cn/api/logbox/oauth2/unifyAccountLogin.do",
//			"Sec-Fetch-Dest": "image",
//			"Sec-Fetch-Mode": "no-cors",
//			"Sec-Fetch-Site": "same-origin",
//		}).Get(u)
//		if err != nil {
//			return err
//		}
//		// Enter the verification code manually
//		//err = message.GetMessenger().WaitSend(message.Message{
//		//	Type:    "image",
//		//	Content: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imgRes.Body()),
//		//}, 10)
//		//if err != nil {
//		//	return err
//		//}
//		//vCodeRS, err = message.GetMessenger().WaitReceive(30)
//		// use ocr api
//		vRes, err := base.RestyClient.R().SetMultipartField(
//			"image", "validateCode.png", "image/png", bytes.NewReader(imgRes.Body())).
//			Post(setting.GetStr(conf.OcrApi))
//		if err != nil {
//			return err
//		}
//		if jsoniter.Get(vRes.Body(), "status").ToInt() != 200 {
//			return errors.New("ocr error:" + jsoniter.Get(vRes.Body(), "msg").ToString())
//		}
//		vCodeRS = jsoniter.Get(vRes.Body(), "result").ToString()
//		log.Debugln("code: ", vCodeRS)
//	}
//	userRsa := RsaEncode([]byte(d.Username), jRsakey, true)
//	passwordRsa := RsaEncode([]byte(d.Password), jRsakey, true)
//	url = "https://open.e.189.cn/api/logbox/oauth2/loginSubmit.do"
//	var loginResp LoginResp
//	res, err = d.client.R().
//		SetHeaders(map[string]string{
//			"lt":         lt,
//			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36",
//			"Referer":    "https://open.e.189.cn/",
//			"accept":     "application/json;charset=UTF-8",
//		}).SetFormData(map[string]string{
//		"appKey":       "cloud",
//		"accountType":  "01",
//		"userName":     "{RSA}" + userRsa,
//		"password":     "{RSA}" + passwordRsa,
//		"validateCode": vCodeRS,
//		"captchaToken": captchaToken,
//		"returnUrl":    returnUrl,
//		"mailSuffix":   "@pan.cn",
//		"paramId":      paramId,
//		"clientType":   "10010",
//		"dynamicCheck": "FALSE",
//		"cb_SaveName":  "1",
//		"isOauth2":     "false",
//	}).Post(url)
//	if err != nil {
//		return err
//	}
//	err = utils.Json.Unmarshal(res.Body(), &loginResp)
//	if err != nil {
//		log.Error(err.Error())
//		return err
//	}
//	if loginResp.Result != 0 {
//		return fmt.Errorf(loginResp.Msg)
//	}
//	_, err = d.client.R().Get(loginResp.ToUrl)
//	return err
//}

func (d *Cloud189) request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	var e Error
	req := d.client.R().SetError(&e).
		SetHeader("Accept", "application/json;charset=UTF-8").
		SetQueryParams(map[string]string{
			"noCache": random(),
		})
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	//log.Debug(res.String())
	if e.ErrorCode != "" {
		if e.ErrorCode == "InvalidSessionKey" {
			err = d.newLogin()
			if err != nil {
				return nil, err
			}
			return d.request(url, method, callback, resp)
		}
	}
	if jsoniter.Get(res.Body(), "res_code").ToInt() != 0 {
		err = errors.New(jsoniter.Get(res.Body(), "res_message").ToString())
	}
	return res.Body(), err
}

func (d *Cloud189) getFiles(fileId string) ([]model.Obj, error) {
	res := make([]model.Obj, 0)
	pageNum := 1
	for {
		var resp Files
		_, err := d.request("https://cloud.189.cn/api/open/file/listFiles.action", http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(map[string]string{
				//"noCache":    random(),
				"pageSize":   "60",
				"pageNum":    strconv.Itoa(pageNum),
				"mediaType":  "0",
				"folderId":   fileId,
				"iconOption": "5",
				"orderBy":    "lastOpTime", //account.OrderBy
				"descending": "true",       //account.OrderDirection
			})
		}, &resp)
		if err != nil {
			return nil, err
		}
		if resp.FileListAO.Count == 0 {
			break
		}
		for _, folder := range resp.FileListAO.FolderList {
			lastOpTime := utils.MustParseCNTime(folder.LastOpTime)
			res = append(res, &model.Object{
				ID:       strconv.FormatInt(folder.Id, 10),
				Name:     folder.Name,
				Modified: lastOpTime,
				IsFolder: true,
			})
		}
		for _, file := range resp.FileListAO.FileList {
			lastOpTime := utils.MustParseCNTime(file.LastOpTime)
			res = append(res, &model.ObjThumb{
				Object: model.Object{
					ID:       strconv.FormatInt(file.Id, 10),
					Name:     file.Name,
					Modified: lastOpTime,
					Size:     file.Size,
				},
				Thumbnail: model.Thumbnail{Thumbnail: file.Icon.SmallUrl},
			})
		}
		pageNum++
	}
	return res, nil
}

func (d *Cloud189) oldUpload(dstDir model.Obj, file model.FileStreamer) error {
	res, err := d.client.R().SetMultipartFormData(map[string]string{
		"parentId":   dstDir.GetID(),
		"sessionKey": "??",
		"opertype":   "1",
		"fname":      file.GetName(),
	}).SetMultipartField("Filedata", file.GetName(), file.GetMimetype(), file).Post("https://hb02.upload.cloud.189.cn/v1/DCIWebUploadAction")
	if err != nil {
		return err
	}
	if utils.Json.Get(res.Body(), "MD5").ToString() != "" {
		return nil
	}
	log.Debugf(res.String())
	return errors.New(res.String())
}

func (d *Cloud189) getSessionKey() (string, error) {
	resp, err := d.request("https://cloud.189.cn/v2/getUserBriefInfo.action", http.MethodGet, nil, nil)
	if err != nil {
		return "", err
	}
	sessionKey := utils.Json.Get(resp, "sessionKey").ToString()
	return sessionKey, nil
}

func (d *Cloud189) getResKey() (string, string, error) {
	now := time.Now().UnixMilli()
	if d.rsa.Expire > now {
		return d.rsa.PubKey, d.rsa.PkId, nil
	}
	resp, err := d.request("https://cloud.189.cn/api/security/generateRsaKey.action", http.MethodGet, nil, nil)
	if err != nil {
		return "", "", err
	}
	pubKey, pkId := utils.Json.Get(resp, "pubKey").ToString(), utils.Json.Get(resp, "pkId").ToString()
	d.rsa.PubKey, d.rsa.PkId = pubKey, pkId
	d.rsa.Expire = utils.Json.Get(resp, "expire").ToInt64()
	return pubKey, pkId, nil
}

func (d *Cloud189) uploadRequest(uri string, form map[string]string, resp interface{}) ([]byte, error) {
	c := strconv.FormatInt(time.Now().UnixMilli(), 10)
	r := Random("xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx")
	l := Random("xxxxxxxxxxxx4xxxyxxxxxxxxxxxxxxx")
	l = l[0 : 16+int(16*myrand.Rand.Float32())]

	e := qs(form)
	data := AesEncrypt([]byte(e), []byte(l[0:16]))
	h := hex.EncodeToString(data)

	sessionKey := d.sessionKey
	signature := hmacSha1(fmt.Sprintf("SessionKey=%s&Operate=GET&RequestURI=%s&Date=%s&params=%s", sessionKey, uri, c, h), l)

	pubKey, pkId, err := d.getResKey()
	if err != nil {
		return nil, err
	}
	b := RsaEncode([]byte(l), pubKey, false)
	req := d.client.R().SetHeaders(map[string]string{
		"accept":         "application/json;charset=UTF-8",
		"SessionKey":     sessionKey,
		"Signature":      signature,
		"X-Request-Date": c,
		"X-Request-ID":   r,
		"EncryptionText": b,
		"PkId":           pkId,
	})
	if resp != nil {
		req.SetResult(resp)
	}
	res, err := req.Get("https://upload.cloud.189.cn" + uri + "?params=" + h)
	if err != nil {
		return nil, err
	}
	data = res.Body()
	if utils.Json.Get(data, "code").ToString() != "SUCCESS" {
		return nil, errors.New(uri + "---" + jsoniter.Get(data, "msg").ToString())
	}
	return data, nil
}

func (d *Cloud189) newUpload(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	sessionKey, err := d.getSessionKey()
	if err != nil {
		return err
	}
	d.sessionKey = sessionKey
	const DEFAULT int64 = 10485760
	var count = int64(math.Ceil(float64(file.GetSize()) / float64(DEFAULT)))

	res, err := d.uploadRequest("/person/initMultiUpload", map[string]string{
		"parentFolderId": dstDir.GetID(),
		"fileName":       encode(file.GetName()),
		"fileSize":       strconv.FormatInt(file.GetSize(), 10),
		"sliceSize":      strconv.FormatInt(DEFAULT, 10),
		"lazyCheck":      "1",
	}, nil)
	if err != nil {
		return err
	}
	uploadFileId := jsoniter.Get(res, "data", "uploadFileId").ToString()
	//_, err = d.uploadRequest("/person/getUploadedPartsInfo", map[string]string{
	//	"uploadFileId": uploadFileId,
	//}, nil)
	var finish int64 = 0
	var i int64
	var byteSize int64
	md5s := make([]string, 0)
	md5Sum := md5.New()
	for i = 1; i <= count; i++ {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		byteSize = file.GetSize() - finish
		if DEFAULT < byteSize {
			byteSize = DEFAULT
		}
		//log.Debugf("%d,%d", byteSize, finish)
		byteData := make([]byte, byteSize)
		n, err := io.ReadFull(file, byteData)
		//log.Debug(err, n)
		if err != nil {
			return err
		}
		finish += int64(n)
		md5Bytes := getMd5(byteData)
		md5Hex := hex.EncodeToString(md5Bytes)
		md5Base64 := base64.StdEncoding.EncodeToString(md5Bytes)
		md5s = append(md5s, strings.ToUpper(md5Hex))
		md5Sum.Write(byteData)
		var resp UploadUrlsResp
		res, err = d.uploadRequest("/person/getMultiUploadUrls", map[string]string{
			"partInfo":     fmt.Sprintf("%s-%s", strconv.FormatInt(i, 10), md5Base64),
			"uploadFileId": uploadFileId,
		}, &resp)
		if err != nil {
			return err
		}
		uploadData := resp.UploadUrls["partNumber_"+strconv.FormatInt(i, 10)]
		log.Debugf("uploadData: %+v", uploadData)
		requestURL := uploadData.RequestURL
		uploadHeaders := strings.Split(decodeURIComponent(uploadData.RequestHeader), "&")
		req, err := http.NewRequest(http.MethodPut, requestURL, driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
		if err != nil {
			return err
		}
		req = req.WithContext(ctx)
		for _, v := range uploadHeaders {
			i := strings.Index(v, "=")
			req.Header.Set(v[0:i], v[i+1:])
		}
		r, err := base.HttpClient.Do(req)
		if err != nil {
			return err
		}
		log.Debugf("%+v %+v", r, r.Request.Header)
		_ = r.Body.Close()
		up(float64(i) * 100 / float64(count))
	}
	fileMd5 := hex.EncodeToString(md5Sum.Sum(nil))
	sliceMd5 := fileMd5
	if file.GetSize() > DEFAULT {
		sliceMd5 = utils.GetMD5EncodeStr(strings.Join(md5s, "\n"))
	}
	res, err = d.uploadRequest("/person/commitMultiUploadFile", map[string]string{
		"uploadFileId": uploadFileId,
		"fileMd5":      fileMd5,
		"sliceMd5":     sliceMd5,
		"lazyCheck":    "1",
		"opertype":     "3",
	}, nil)
	return err
}
