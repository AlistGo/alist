package doubao_new

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	driver.RootID
	// define other
	Authorization string `json:"authorization" help:"DPoP access token (Authorization header value); optional if present in cookie"`
	Dpop          string `json:"dpop" help:"DPoP header value; optional if present in cookie"`
	Cookie        string `json:"cookie" help:"Optional cookie; only used to extract authorization/dpop tokens"`
	Debug         bool   `json:"debug" help:"Enable debug logs for upload"`
}

var config = driver.Config{
	Name:              "DoubaoNew",
	LocalSort:         true,
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
		return &DoubaoNew{}
	})
}
