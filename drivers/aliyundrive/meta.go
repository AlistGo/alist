package aliyundrive

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	RefreshToken   string `json:"refresh_token" required:"true"`
	DeviceID       string `json:"device_id" required:"true"`
	OrderBy        string `json:"order_by" type:"select" options:"name,size,updated_at,created_at"`
	OrderDirection string `json:"order_direction" type:"select" options:"ASC,DESC"`
	RapidUpload    bool   `json:"rapid_upload"`
	InternalUpload bool   `json:"internal_upload"`
}

var config = driver.Config{
	Name:        "Aliyundrive",
	DefaultRoot: "root",
	Alert: `warning|There may be an infinite loop bug in this driver.
Deprecated, no longer maintained and will be removed in a future version.
We recommend using the official driver AliyundriveOpen.`,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &AliDrive{}
	})
}
