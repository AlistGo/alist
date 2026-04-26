package _139

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Authorization string `json:"authorization" type:"text" required:"true"`
	driver.RootID
	Type                 string `json:"type" type:"select" options:"personal_new,family,group,personal,share" default:"personal_new"`
	CloudID              string `json:"cloud_id"`
	LinkID               string `json:"link_id"`
	CustomUploadPartSize int64  `json:"custom_upload_part_size" type:"number" default:"0"`
	ReportRealSize       bool   `json:"report_real_size" type:"bool" default:"true"`
	UseLargeThumbnail    bool   `json:"use_large_thumbnail" type:"bool" default:"false"`
}

var config = driver.Config{
	Name:             "139Yun",
	LocalSort:        true,
	ProxyRangeOption: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		d := &Yun139{}
		d.ProxyRange = true
		return d
	})
}
