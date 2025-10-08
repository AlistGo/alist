package gofile

import (
    "github.com/alist-org/alist/v3/internal/driver"
    "github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
    driver.RootID
    APIToken         string `json:"api_token" required:"true" help:"Get your API token from your Gofile profile page"`
    LinkExpiry       int    `json:"link_expiry" type:"number" default:"30" help:"Direct link cache duration in days. Set to 0 to disable caching"`
    DirectLinkExpiry int    `json:"direct_link_expiry" type:"number" default:"0" help:"Direct link expiration time in hours on Gofile server. Set to 0 for no expiration"`
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
