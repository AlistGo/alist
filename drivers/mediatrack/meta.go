package mediatrack

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	AccessToken string `json:"access_token" required:"true"`
	ProjectID   string `json:"project_id"`
	driver.RootID
	OrderBy           string `json:"order_by" type:"select" options:"updated_at,title,size" default:"title"`
	OrderDesc         bool   `json:"order_desc"`
	DeviceFingerprint string `json:"device_fingerprint" required:"true"`
}

var config = driver.Config{
	Name: "MediaTrack",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &MediaTrack{}
	})
}
