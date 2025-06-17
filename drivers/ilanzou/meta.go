package template

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	Username string `json:"username" type:"string" required:"true"`
	Password string `json:"password" type:"string" required:"true"`
	Ip       string `json:"ip" type:"string"`

	Token string
	UUID  string
}

type Conf struct {
	base       string
	secret     []byte
	bucket     string
	unproved   string
	proved     string
	devVersion string
	site       string
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ILanZou{
			config: driver.Config{
				Name:              "ILanZou",
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
			},
			conf: Conf{
				base:       "https://api.ilanzou.com",
				secret:     []byte("lanZouY-disk-app"),
				bucket:     "wpanstore-lanzou",
				unproved:   "unproved",
				proved:     "proved",
				devVersion: "125",
				site:       "https://www.ilanzou.com",
			},
		}
	})
	op.RegisterDriver(func() driver.Driver {
		return &ILanZou{
			config: driver.Config{
				Name:              "FeijiPan",
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
			},
			conf: Conf{
				base:       "https://api.feijipan.com",
				secret:     []byte("dingHao-disk-app"),
				bucket:     "wpanstore",
				unproved:   "ws",
				proved:     "app",
				devVersion: "125",
				site:       "https://www.feijipan.com",
			},
		}
	})
}
