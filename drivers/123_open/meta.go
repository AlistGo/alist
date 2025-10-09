package _123Open

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID

	ClientID      string `json:"client_id" required:"true" label:"Client ID"`
	ClientSecret  string `json:"client_secret" required:"true" label:"Client Secret"`
	PrivateKey    string `json:"private_key"`
	UID           uint64 `json:"uid" type:"number"`
	ValidDuration int64  `json:"valid_duration" type:"number" default:"30" help:"minutes"`
}

var config = driver.Config{
	Name:              "123 Open",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "0",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Open123{}
	})
}
