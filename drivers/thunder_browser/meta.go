package thunder_browser

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
)

// ExpertAddition 高级设置
type ExpertAddition struct {
	driver.RootID

	LoginType string `json:"login_type" type:"select" options:"user,refresh_token" default:"user"`
	SignType  string `json:"sign_type" type:"select" options:"algorithms,captcha_sign" default:"algorithms"`

	// 登录方式1
	Username string `json:"username" required:"true" help:"login type is user,this is required"`
	Password string `json:"password" required:"true" help:"login type is user,this is required"`
	// 登录方式2
	RefreshToken string `json:"refresh_token" required:"true" help:"login type is refresh_token,this is required"`

	SafePassword string `json:"safe_password" required:"true" help:"super safe password"` // 超级保险箱密码

	// 签名方法1
	Algorithms string `json:"algorithms" required:"true" help:"sign type is algorithms,this is required" default:"Cw4kArmKJ/aOiFTxnQ0ES+D4mbbrIUsFn,HIGg0Qfbpm5ThZ/RJfjoao4YwgT9/M,u/PUD,OlAm8tPkOF1qO5bXxRN2iFttuDldrg,FFIiM6sFhWhU7tIMVUKOF7CUv/KzgwwV8FE,yN,4m5mglrIHksI6wYdq,LXEfS7,T+p+C+F2yjgsUtiXWU/cMNYEtJI4pq7GofW,14BrGIEMXkbvFvZ49nDUfVCRcHYFOJ1BP1Y,kWIH3Row,RAmRTKNCjucPWC"`
	// 签名方法2
	CaptchaSign string `json:"captcha_sign" required:"true" help:"sign type is captcha_sign,this is required"`
	Timestamp   string `json:"timestamp" required:"true" help:"sign type is captcha_sign,this is required"`

	// 验证码
	CaptchaToken string `json:"captcha_token"`
	// 信任密钥
	CreditKey string `json:"credit_key" help:"credit key,used for login"`

	// 必要且影响登录,由签名决定
	DeviceID      string `json:"device_id"  required:"false" default:""`
	ClientID      string `json:"client_id"  required:"true" default:"ZUBzD9J_XPXfn7f7"`
	ClientSecret  string `json:"client_secret"  required:"true" default:"yESVmHecEe6F0aou69vl-g"`
	ClientVersion string `json:"client_version"  required:"true" default:"1.40.0.7208"`
	PackageName   string `json:"package_name"  required:"true" default:"com.xunlei.browser"`

	// 不影响登录,影响下载速度
	UserAgent         string `json:"user_agent"  required:"false" default:""`
	DownloadUserAgent string `json:"download_user_agent"  required:"false" default:""`

	// 优先使用视频链接代替下载链接
	UseVideoUrl bool `json:"use_video_url"`
	// 移除方式
	RemoveWay string `json:"remove_way" required:"true" type:"select" options:"trash,delete"`
}

// GetIdentity 登录特征,用于判断是否重新登录
func (i *ExpertAddition) GetIdentity() string {
	hash := md5.New()
	if i.LoginType == "refresh_token" {
		hash.Write([]byte(i.RefreshToken))
	} else {
		hash.Write([]byte(i.Username + i.Password))
	}

	if i.SignType == "captcha_sign" {
		hash.Write([]byte(i.CaptchaSign + i.Timestamp))
	} else {
		hash.Write([]byte(i.Algorithms))
	}

	hash.Write([]byte(i.DeviceID))
	hash.Write([]byte(i.ClientID))
	hash.Write([]byte(i.ClientSecret))
	hash.Write([]byte(i.ClientVersion))
	hash.Write([]byte(i.PackageName))
	return hex.EncodeToString(hash.Sum(nil))
}

type Addition struct {
	driver.RootID
	Username     string `json:"username" required:"true"`
	Password     string `json:"password" required:"true"`
	SafePassword string `json:"safe_password" required:"true"` // 超级保险箱密码
	CaptchaToken string `json:"captcha_token"`
	CreditKey    string `json:"credit_key" help:"credit key,used for login"` // 信任密钥
	DeviceID     string `json:"device_id" default:""`                        // 登录设备ID
	UseVideoUrl  bool   `json:"use_video_url" default:"false"`
	RemoveWay    string `json:"remove_way" required:"true" type:"select" options:"trash,delete"`
}

// GetIdentity 登录特征,用于判断是否重新登录
func (i *Addition) GetIdentity() string {
	return utils.GetMD5EncodeStr(i.Username + i.Password)
}

var config = driver.Config{
	Name:      "ThunderBrowser",
	LocalSort: true,
}

var configExpert = driver.Config{
	Name:      "ThunderBrowserExpert",
	LocalSort: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ThunderBrowser{}
	})
	op.RegisterDriver(func() driver.Driver {
		return &ThunderBrowserExpert{}
	})
}
