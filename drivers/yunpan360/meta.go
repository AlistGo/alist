package yunpan360

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	AuthType       string `json:"auth_type" type:"select" options:"cookie,api_key" default:"cookie"`
	Cookie         string `json:"cookie" type:"text" help:"Cookie copied from a logged-in yunpan.com session; used when auth_type=cookie"`
	OwnerQID       string `json:"owner_qid" type:"text" help:"Optional owner_qid for cookie-mode download; leave empty to auto-detect"`
	DownloadToken  string `json:"download_token" type:"text" help:"Optional web token for cookie-mode download; leave empty to auto-detect"`
	APIKey         string `json:"api_key" type:"text" help:"360 AI YunPan API key; used when auth_type=api_key"`
	EcsEnv         string `json:"ecs_env" type:"select" options:"prod,test,hgtest" default:"prod"`
	SubChannel     string `json:"sub_channel" default:"open"`
	OrderDirection string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	PageSize       int    `json:"page_size" type:"number" default:"100" help:"List page size"`
}

var config = driver.Config{
	Name:        "360AIYunPan",
	LocalSort:   false,
	CheckStatus: true,
	NoUpload:    false,
	DefaultRoot: "/",
	Alert:       "info|api_key mode supports list/link/upload/mkdir/rename/move/delete; cookie mode supports list/link/mkdir/rename/move/delete only, and forces web proxy because direct download URLs require web headers.",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Yunpan360{}
	})
}
