package emby

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Leave root_folder_id empty to list all library views
	driver.RootID
	Address  string `json:"address" required:"true" help:"Emby/Jellyfin server address, e.g. http://192.168.1.1:8096"`
	Username string `json:"username" required:"true"`
	Password string `json:"password"`
}

var config = driver.Config{
	Name:        "Emby",
	LocalSort:   true,
	NoUpload:    true,
	DefaultRoot: "",
	Alert:       "info|Direct links contain your Emby access token. Enable proxy for this storage if it is shared publicly.",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Emby{}
	})
}
