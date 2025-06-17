package febbox

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	ClientID     string `json:"client_id" required:"true" default:""`
	ClientSecret string `json:"client_secret" required:"true" default:""`
	RefreshToken string
	SortRule     string `json:"sort_rule" required:"true" type:"select" options:"size_asc,size_desc,name_asc,name_desc,update_asc,update_desc,ext_asc,ext_desc" default:"name_asc"`
	PageSize     int64  `json:"page_size" required:"true" type:"number" default:"100" help:"list api per page size of FebBox driver"`
	UserIP       string `json:"user_ip" default:"" help:"user ip address for download link which can speed up the download"`
}

var config = driver.Config{
	Name:              "FebBox",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          true,
	NeedMs:            false,
	DefaultRoot:       "0",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &FebBox{}
	})
}
