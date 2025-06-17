package pikpak_share

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/pkg/utils"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

var AndroidAlgorithms = []string{
	"SOP04dGzk0TNO7t7t9ekDbAmx+eq0OI1ovEx",
	"nVBjhYiND4hZ2NCGyV5beamIr7k6ifAsAbl",
	"Ddjpt5B/Cit6EDq2a6cXgxY9lkEIOw4yC1GDF28KrA",
	"VVCogcmSNIVvgV6U+AochorydiSymi68YVNGiz",
	"u5ujk5sM62gpJOsB/1Gu/zsfgfZO",
	"dXYIiBOAHZgzSruaQ2Nhrqc2im",
	"z5jUTBSIpBN9g4qSJGlidNAutX6",
	"KJE2oveZ34du/g1tiimm",
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
	AndroidClientID      = "YNxT9w7GMdWvEOKa"
	AndroidClientSecret  = "dbw2OtmVEeuUvIptb1Coyg"
	AndroidClientVersion = "1.53.2"
	AndroidPackageName   = "com.pikcloud.pikpak"
	AndroidSdkVersion    = "2.0.6.206003"
	WebClientID          = "YUMx5nI8ZU8Ap8pm"
	WebClientSecret      = "dbw2OtmVEeuUvIptb1Coyg"
	WebClientVersion     = "2.0.0"
	WebPackageName       = "mypikpak.com"
	WebSdkVersion        = "8.0.3"
	PCClientID           = "YvtoWO6GNHiuCl7x"
	PCClientSecret       = "1NIH5R1IEe2pAxZE3hv3uA"
	PCClientVersion      = "undefined" // 2.6.11.4955
	PCPackageName        = "mypikpak.com"
	PCSdkVersion         = "8.0.3"
)

func (d *PikPakShare) request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()
	req.SetHeaders(map[string]string{
		"User-Agent":      d.GetUserAgent(),
		"X-Client-ID":     d.GetClientID(),
		"X-Device-ID":     d.GetDeviceID(),
		"X-Captcha-Token": d.GetCaptchaToken(),
	})

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
	case 9: // 验证码token过期
		if err = d.RefreshCaptchaToken(GetAction(method, url), ""); err != nil {
			return nil, err
		}
		return d.request(url, method, callback, resp)
	case 10: // 操作频繁
		return nil, errors.New(e.ErrorDescription)
	default:
		return nil, errors.New(e.Error())
	}
}

func (d *PikPakShare) getSharePassToken() error {
	query := map[string]string{
		"share_id":       d.ShareId,
		"pass_code":      d.SharePwd,
		"thumbnail_size": "SIZE_LARGE",
		"limit":          "100",
	}
	var resp ShareResp
	_, err := d.request("https://api-drive.mypikpak.net/drive/v1/share", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(query)
	}, &resp)
	if err != nil {
		return err
	}
	d.PassCodeToken = resp.PassCodeToken
	return nil
}

func (d *PikPakShare) getFiles(id string) ([]File, error) {
	res := make([]File, 0)
	pageToken := "first"
	for pageToken != "" {
		if pageToken == "first" {
			pageToken = ""
		}
		query := map[string]string{
			"parent_id":       id,
			"share_id":        d.ShareId,
			"thumbnail_size":  "SIZE_LARGE",
			"with_audit":      "true",
			"limit":           "100",
			"filters":         `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
			"page_token":      pageToken,
			"pass_code_token": d.PassCodeToken,
		}
		var resp ShareResp
		_, err := d.request("https://api-drive.mypikpak.net/drive/v1/share/detail", http.MethodGet, func(req *resty.Request) {
			req.SetQueryParams(query)
		}, &resp)
		if err != nil {
			return nil, err
		}
		if resp.ShareStatus != "OK" {
			if resp.ShareStatus == "PASS_CODE_EMPTY" || resp.ShareStatus == "PASS_CODE_ERROR" {
				err = d.getSharePassToken()
				if err != nil {
					return nil, err
				}
				return d.getFiles(id)
			}
			return nil, errors.New(resp.ShareStatusText)
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
}

func (c *Common) SetUserAgent(userAgent string) {
	c.UserAgent = userAgent
}

func (c *Common) SetCaptchaToken(captchaToken string) {
	c.CaptchaToken = captchaToken
}

func (c *Common) SetDeviceID(deviceID string) {
	c.DeviceID = deviceID
}

func (c *Common) GetCaptchaToken() string {
	return c.CaptchaToken
}

func (c *Common) GetClientID() string {
	return c.ClientID
}

func (c *Common) GetUserAgent() string {
	return c.UserAgent
}

func (c *Common) GetDeviceID() string {
	return c.DeviceID
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

// RefreshCaptchaToken 刷新验证码token
func (d *PikPakShare) RefreshCaptchaToken(action, userID string) error {
	metas := map[string]string{
		"client_version": d.ClientVersion,
		"package_name":   d.PackageName,
		"user_id":        userID,
	}
	metas["timestamp"], metas["captcha_sign"] = d.Common.GetCaptchaSign()
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
func (d *PikPakShare) refreshCaptchaToken(action string, metas map[string]string) error {
	param := CaptchaTokenRequest{
		Action:       action,
		CaptchaToken: d.GetCaptchaToken(),
		ClientID:     d.ClientID,
		DeviceID:     d.GetDeviceID(),
		Meta:         metas,
	}
	var e ErrResp
	var resp CaptchaTokenResponse
	_, err := d.request("https://user.mypikpak.net/v1/shield/captcha/init", http.MethodPost, func(req *resty.Request) {
		req.SetError(&e).SetBody(param)
	}, &resp)

	if err != nil {
		return err
	}

	if e.IsError() {
		return errors.New(e.Error())
	}

	//if resp.Url != "" {
	//	return fmt.Errorf(`need verify: <a target="_blank" href="%s">Click Here</a>`, resp.Url)
	//}

	if d.Common.RefreshCTokenCk != nil {
		d.Common.RefreshCTokenCk(resp.CaptchaToken)
	}
	d.Common.SetCaptchaToken(resp.CaptchaToken)
	return nil
}
