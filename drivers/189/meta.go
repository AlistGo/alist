package _189

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Username   string `json:"username" required:"true"`
	Password   string `json:"password" required:"true"`
	Cookie     string `json:"cookie" help:"Fill in the cookie if need captcha"`
	StripEmoji bool   `json:"strip_emoji" help:"Remove four-byte characters (e.g., emoji) before upload"`
	driver.RootID
}

var config = driver.Config{
	Name:        "189Cloud",
	LocalSort:   true,
	DefaultRoot: "-11",
	Alert:       `info|You can try to use 189PC driver if this driver does not work.`,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Cloud189{}
	})
}
