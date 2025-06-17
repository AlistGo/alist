package pikpak_share

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	ShareId               string `json:"share_id" required:"true"`
	SharePwd              string `json:"share_pwd"`
	Platform              string `json:"platform" default:"web" required:"true" type:"select" options:"android,web,pc"`
	DeviceID              string `json:"device_id"  required:"false" default:""`
	UseTransCodingAddress bool   `json:"use_transcoding_address" required:"true" default:"false"`
}

var config = driver.Config{
	Name:        "PikPakShare",
	LocalSort:   true,
	NoUpload:    true,
	DefaultRoot: "",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &PikPakShare{}
	})
}
