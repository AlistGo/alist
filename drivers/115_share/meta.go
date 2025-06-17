package _115_share

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Cookie       string  `json:"cookie" type:"text" help:"one of QR code token and cookie required"`
	QRCodeToken  string  `json:"qrcode_token" type:"text" help:"one of QR code token and cookie required"`
	QRCodeSource string  `json:"qrcode_source" type:"select" options:"web,android,ios,tv,alipaymini,wechatmini,qandroid" default:"linux" help:"select the QR code device, default linux"`
	PageSize     int64   `json:"page_size" type:"number" default:"1000" help:"list api per page size of 115 driver"`
	LimitRate    float64 `json:"limit_rate" type:"float" default:"2" help:"limit all api request rate (1r/[limit_rate]s)"`
	ShareCode    string  `json:"share_code" type:"text" required:"true" help:"share code of 115 share link"`
	ReceiveCode  string  `json:"receive_code" type:"text" required:"true" help:"receive code of 115 share link"`
	driver.RootID
}

var config = driver.Config{
	Name:        "115 Share",
	DefaultRoot: "0",
	// OnlyProxy:   true,
	// OnlyLocal:         true,
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: true,
	NoUpload:          true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Pan115Share{}
	})
}
