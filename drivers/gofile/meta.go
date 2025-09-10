package gofile

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	APIToken string `json:"api_token" required:"true" help:"Get your API token from your Gofile profile page"`
}

var config = driver.Config{
	Name:        "Gofile",
	DefaultRoot: "",
	LocalSort:   false,
	OnlyProxy:   false,
	NoCache:     false,
	NoUpload:    false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Gofile{}
	})
}