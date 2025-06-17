package baidu_netdisk

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	RefreshToken string `json:"refresh_token" required:"true"`
	driver.RootPath
	OrderBy               string `json:"order_by" type:"select" options:"name,time,size" default:"name"`
	OrderDirection        string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	DownloadAPI           string `json:"download_api" type:"select" options:"official,crack,crack_video" default:"official"`
	ClientID              string `json:"client_id" required:"true" default:"iYCeC9g08h5vuP9UqvPHKKSVrKFXGa1v"`
	ClientSecret          string `json:"client_secret" required:"true" default:"jXiFMOPVPCWlO2M5CwWQzffpNPaGTRBG"`
	CustomCrackUA         string `json:"custom_crack_ua" required:"true" default:"netdisk"`
	AccessToken           string
	UploadThread          string `json:"upload_thread" default:"3" help:"1<=thread<=32"`
	UploadAPI             string `json:"upload_api" default:"https://d.pcs.baidu.com"`
	CustomUploadPartSize  int64  `json:"custom_upload_part_size" type:"number" default:"0" help:"0 for auto"`
	LowBandwithUploadMode bool   `json:"low_bandwith_upload_mode" default:"false"`
	OnlyListVideoFile     bool   `json:"only_list_video_file" default:"false"`
}

var config = driver.Config{
	Name:        "BaiduNetdisk",
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &BaiduNetdisk{}
	})
}
