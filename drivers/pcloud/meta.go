package pcloud

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Using json tag "access_token" for UI display, but internally it's a refresh token
	RefreshToken   string `json:"access_token" required:"true" help:"OAuth token from pCloud authorization"`
	Hostname       string `json:"hostname" type:"select" options:"us,eu" default:"us" help:"Select pCloud server region"`
	RootFolderID   string `json:"root_folder_id" help:"Get folder ID from URL like https://my.pcloud.com/#/filemanager?folder=12345678901 (leave empty for root folder)"`
	ClientID       string `json:"client_id" help:"Custom OAuth client ID (optional)"`
	ClientSecret   string `json:"client_secret" help:"Custom OAuth client secret (optional)"`
}

// Implement IRootId interface
func (a Addition) GetRootId() string {
	return a.RootFolderID
}

var config = driver.Config{
	Name:        "pCloud",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &PCloud{}
	})
}