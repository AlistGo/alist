package _123

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Username     string `json:"username" required:"true"`
	Password     string `json:"password" required:"true"`
	SafePassword string `json:"safe_password"`
	driver.RootID
	//OrderBy        string `json:"order_by" type:"select" options:"file_id,file_name,size,update_at" default:"file_name"`
	//OrderDirection string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	AccessToken string
}

var config = driver.Config{
	Name:        "123Pan",
	DefaultRoot: "0",
	LocalSort:   true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Pan123{}
	})
}
