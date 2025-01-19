package github_releases

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	RepoStructure string `json:"repo_structure" type:"text" required:"true" default:"/path/to/alist-gh:alistGo/alist\n/path/to2/alist-web-gh:AlistGo/alist-web" help:"structure:[path:]org/repo"`
	ShowReadme    bool   `json:"show_readme" type:"bool" default:"true" help:"show README、LICENSE file"`
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
