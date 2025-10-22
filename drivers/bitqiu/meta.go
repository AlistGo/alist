package bitqiu

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	Username     string `json:"username" required:"true"`
	Password     string `json:"password" required:"true"`
	UserPlatform string `json:"user_platform" help:"Optional device identifier; auto-generated if empty."`
	OrderType    string `json:"order_type" type:"select" options:"updateTime,createTime,name,size" default:"updateTime"`
	OrderDesc    bool   `json:"order_desc"`
	PageSize     string `json:"page_size" default:"24" help:"Number of entries to request per page."`
}

var config = driver.Config{
	Name:        "BitQiu",
	DefaultRoot: "0",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &BitQiu{}
	})
}
