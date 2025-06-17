package alist_v3

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	Address           string `json:"url" required:"true"`
	MetaPassword      string `json:"meta_password"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	Token             string `json:"token"`
	PassUAToUpsteam   bool   `json:"pass_ua_to_upsteam" default:"true"`
	ForwardArchiveReq bool   `json:"forward_archive_requests" default:"true"`
}

var config = driver.Config{
	Name:             "AList V3",
	LocalSort:        true,
	DefaultRoot:      "/",
	CheckStatus:      true,
	ProxyRangeOption: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &AListV3{}
	})
}
