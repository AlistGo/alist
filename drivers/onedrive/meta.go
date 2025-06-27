package onedrive

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	Region       string `json:"region" type:"select" required:"true" options:"global,cn,us,de" default:"global"`
	IsSharepoint bool   `json:"is_sharepoint"`
	ClientID     string `json:"client_id" required:"true"`
	ClientSecret string `json:"client_secret" required:"true"`
	RedirectUri  string `json:"redirect_uri" required:"true" default:"https://alistgo.com/tool/onedrive/callback"`
	RefreshToken string `json:"refresh_token" required:"true"`
	SiteId       string `json:"site_id"`
	ChunkSize    int64  `json:"chunk_size" type:"number" default:"5"`
	CustomHost   string `json:"custom_host" help:"Custom host for onedrive download link"`
}

var config = driver.Config{
	Name:        "Onedrive",
	LocalSort:   true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Onedrive{}
	})
}
