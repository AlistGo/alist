package github_releases

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	RepoStructure  string `json:"repo_structure" type:"text" required:"true" default:"alistGo/alist" help:"structure:[path:]org/repo"`
	ShowReadme     bool   `json:"show_readme" type:"bool" default:"true" help:"show README、LICENSE file"`
	Token          string `json:"token" type:"string" required:"false" help:"GitHub token, if you want to access private repositories or increase the rate limit"`
	ShowAllVersion bool   `json:"show_all_version" type:"bool" default:"false" help:"show all versions"`
	GitHubProxy    string `json:"gh_proxy" type:"string" default:"" help:"GitHub proxy, e.g. https://ghproxy.net/github.com or https://gh-proxy.com/github.com "`
}

var config = driver.Config{
	Name:              "GitHub Releases",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &GithubReleases{}
	})
}
