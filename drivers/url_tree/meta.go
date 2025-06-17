package url_tree

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	// driver.RootPath
	// driver.RootID
	// define other
	UrlStructure string `json:"url_structure" type:"text" required:"true" default:"https://jsd.nn.ci/gh/alist-org/alist/README.md\nhttps://jsd.nn.ci/gh/alist-org/alist/README_cn.md\nfolder:\n  CONTRIBUTING.md:1635:https://jsd.nn.ci/gh/alist-org/alist/CONTRIBUTING.md\n  CODE_OF_CONDUCT.md:2093:https://jsd.nn.ci/gh/alist-org/alist/CODE_OF_CONDUCT.md" help:"structure:FolderName:\n  [FileName:][FileSize:][Modified:]Url"`
	HeadSize     bool   `json:"head_size" type:"bool" default:"false" help:"Use head method to get file size, but it may be failed."`
	Writable     bool   `json:"writable" type:"bool" default:"false"`
}

var config = driver.Config{
	Name:              "UrlTree",
	LocalSort:         true,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           true,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "",
	CheckStatus:       true,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Urls{}
	})
}
