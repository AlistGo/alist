package gitee

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	Endpoint      string `json:"endpoint" type:"string" help:"Gitee API endpoint, default https://gitee.com/api/v5"`
	Token         string `json:"token" type:"string"`
	Owner         string `json:"owner" type:"string" required:"true"`
	Repo          string `json:"repo" type:"string" required:"true"`
	Ref           string `json:"ref" type:"string" help:"Branch, tag or commit SHA, defaults to repository default branch"`
	DownloadProxy string `json:"download_proxy" type:"string" help:"Prefix added before download URLs, e.g. https://mirror.example.com/"`
	Cookie        string `json:"cookie" type:"string" help:"Cookie returned from user info request"`
}

var config = driver.Config{
	Name:        "Gitee",
	LocalSort:   true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Gitee{}
	})
}
