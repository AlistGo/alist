package _123_open

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	RefreshToken string `json:"RefreshToken" required:"false"`
	ClientID     string `json:"ClientID" required:"false"`
	ClientSecret string `json:"ClientSecret" required:"false"`
	AccessToken  string `json:"AccessToken" required:"false"`
	UploadThread int    `json:"UploadThread" type:"number" default:"3" help:"the threads of upload"`
}

var config = driver.Config{
	Name:        "123 Open",
	DefaultRoot: "0",
	LocalSort:   true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Open123{}
	})
}
