package baiduphoto

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// RefreshToken string `json:"refresh_token" required:"true"`
	Cookie   string `json:"cookie" required:"true"`
	ShowType string `json:"show_type" type:"select" options:"root,root_only_album,root_only_file" default:"root"`
	AlbumID  string `json:"album_id"`
	//AlbumPassword string `json:"album_password"`
	DeleteOrigin bool `json:"delete_origin"`
	// ClientID     string `json:"client_id" required:"true" default:"iYCeC9g08h5vuP9UqvPHKKSVrKFXGa1v"`
	// ClientSecret string `json:"client_secret" required:"true" default:"jXiFMOPVPCWlO2M5CwWQzffpNPaGTRBG"`
	UploadThread string `json:"upload_thread" default:"3" help:"1<=thread<=32"`
}

var config = driver.Config{
	Name:      "BaiduPhoto",
	LocalSort: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &BaiduPhoto{}
	})
}
