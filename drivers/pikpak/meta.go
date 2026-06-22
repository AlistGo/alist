package pikpak

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	Username         string `json:"username" required:"true"`
	Password         string `json:"password" required:"true"`
	Platform         string `json:"platform" required:"true" default:"web" type:"select" options:"android,web,pc"`
	RefreshToken     string `json:"refresh_token" required:"true" default:""`
	CaptchaToken     string `json:"captcha_token" default:""`
	DeviceID         string `json:"device_id"  required:"false" default:""`
	DisableMediaLink bool   `json:"disable_media_link" default:"true"`
	// API 请求域名：从内置列表选择，或用 custom_api_domain 手动填写覆盖。用于构建 api-drive.<domain> / user.<domain>
	ApiDomain       string `json:"api_domain" type:"select" options:"mypikpak_net,mypikpak_com,pikpak_me,pikpakdrive_com" default:"mypikpak_net"`
	CustomApiDomain string `json:"custom_api_domain" help:"Custom PikPak API domain (e.g. mypikpak.net). When set, it overrides the selected api_domain."`
	// 下载/播放直链域名：original=不改写；或从内置列表选择 / 用 custom_download_domain 手动填写覆盖
	DownloadDomain       string `json:"download_domain" type:"select" options:"original,mypikpak_net,mypikpak_com,pikpak_me,pikpakdrive_com" default:"original"`
	CustomDownloadDomain string `json:"custom_download_domain" help:"Custom domain to rewrite the download/play link host to (e.g. mypikpak.net). When set, it overrides the selected download_domain. Keep 'original' (and empty) to leave the link unchanged."`
}

var config = driver.Config{
	Name:        "PikPak",
	LocalSort:   true,
	DefaultRoot: "",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &PikPak{}
	})
}
