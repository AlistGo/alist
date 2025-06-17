package misskey

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	driver.RootPath
	// define other
	// Field string `json:"field" type:"select" required:"true" options:"a,b,c" default:"a"`
	Endpoint    string `json:"endpoint" required:"true" default:"https://misskey.io"`
	AccessToken string `json:"access_token" required:"true"`
}

var config = driver.Config{
	Name:              "Misskey",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "/",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Misskey{}
	})
}
