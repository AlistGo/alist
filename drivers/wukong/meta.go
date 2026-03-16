package wukong

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	Cookie   string `json:"cookie" type:"text" required:"true" help:"Cookie from https://pan.wkbrowser.com/"`
	Aid      string `json:"aid" default:"590353" help:"aid query param used by web requests"`
	Language string `json:"language" default:"zh"`
	PageSize int    `json:"page_size" type:"number" default:"100"`
}

var config = driver.Config{
	Name:              "WuKongNetdisk",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "0",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Wukong{}
	})
}
