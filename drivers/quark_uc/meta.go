package quark

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Cookie string `json:"cookie" required:"true"`
	driver.RootID
	OrderBy               string `json:"order_by" type:"select" options:"none,file_type,file_name,updated_at" default:"none"`
	OrderDirection        string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	UseTransCodingAddress bool   `json:"use_transcoding_address" help:"You can watch the transcoded video and support 302 redirection" required:"true" default:"false"`
	OnlyListVideoFile     bool   `json:"only_list_video_file" default:"false"`
	AdditionVersion       int
}

type Conf struct {
	ua      string
	referer string
	api     string
	pr      string
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &QuarkOrUC{
			config: driver.Config{
				Name:              "Quark",
				DefaultRoot:       "0",
				NoOverwriteUpload: true,
			},
			conf: Conf{
				ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch",
				referer: "https://pan.quark.cn",
				api:     "https://drive.quark.cn/1/clouddrive",
				pr:      "ucpro",
			},
		}
	})
	op.RegisterDriver(func() driver.Driver {
		return &QuarkOrUC{
			config: driver.Config{
				Name:              "UC",
				OnlyProxy:         true,
				DefaultRoot:       "0",
				NoOverwriteUpload: true,
			},
			conf: Conf{
				ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) uc-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch",
				referer: "https://drive.uc.cn",
				api:     "https://pc-api.uc.cn/1/clouddrive",
				pr:      "UCBrowser",
			},
		}
	})
}
