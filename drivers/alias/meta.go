package alias

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	// driver.RootPath
	// define other
	Paths               string `json:"paths" required:"true" type:"text"`
	ProtectSameName     bool   `json:"protect_same_name" default:"true" required:"false" help:"Protects same-name files from Delete or Rename"`
	DownloadConcurrency int    `json:"download_concurrency" default:"0" required:"false" type:"number" help:"Need to enable proxy"`
	DownloadPartSize    int    `json:"download_part_size" default:"0" type:"number" required:"false" help:"Need to enable proxy. Unit: KB"`
	Writable            bool   `json:"writable" type:"bool" default:"false"`
}

var config = driver.Config{
	Name:             "Alias",
	LocalSort:        true,
	NoCache:          true,
	NoUpload:         false,
	DefaultRoot:      "/",
	ProxyRangeOption: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Alias{
			Addition: Addition{
				ProtectSameName: true,
			},
		}
	})
}
