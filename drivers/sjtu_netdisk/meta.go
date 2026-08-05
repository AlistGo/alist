package sjtu_netdisk

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	OrderBy     string `json:"order_by" type:"select" required:"true" options:"name,modificationTime,size" default:"name"`
	OrderByType string `json:"order_by_type" type:"select" required:"true" options:"asc,desc" default:"asc"`
	UserToken   string `json:"user_token" required:"true"`
	UserId      string `json:"user_id" required:"true"`
	KeepAlive   string `json:"keep_alive" required:"true"`
}

var config = driver.Config{
	Name:              "SJTUNetdisk",
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

var API_URL = "https://pan.sjtu.edu.cn/api/v1"

// used by refreshToken
var TOKEN_URL = "https://pan.sjtu.edu.cn/user/v1/space/1/personal"

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &SJTUNetdisk{}
	})
}
