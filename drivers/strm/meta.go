package strm

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

const (
	SaveLocalInsertMode = "insert"
	SaveLocalUpdateMode = "update"
	SaveLocalSyncMode   = "sync"
)

type Addition struct {
	Paths             string `json:"paths" required:"true" type:"text"`
	SiteUrl           string `json:"siteUrl" type:"text" required:"false" help:"The prefix URL of generated strm file"`
	PathPrefix        string `json:"PathPrefix" type:"text" required:"false" default:"/d" help:"Path prefix in strm content"`
	DownloadFileTypes string `json:"downloadFileTypes" type:"text" default:"ass,srt,vtt,sub,strm" required:"false" help:"Extensions to download as local files"`
	FilterFileTypes   string `json:"filterFileTypes" type:"text" default:"mp4,mkv,flv,avi,wmv,ts,rmvb,webm,mp3,flac,aac,wav,ogg,m4a,wma,alac" required:"false" help:"Extensions to expose as .strm"`
	EncodePath        bool   `json:"encodePath" default:"true" required:"true" help:"Encode path in strm content"`
	WithoutUrl        bool   `json:"withoutUrl" default:"false" help:"Generate path-only strm content"`
	WithSign          bool   `json:"withSign" default:"false" help:"Append sign query to generated URL"`
	SignExpireHours   int    `json:"SignExpireHours" type:"number" default:"0" help:"Driver-level sign expiration in hours. 0 uses global link_expiration"`
	RotateSignNow     bool   `json:"RotateSignNow" type:"bool" default:"false" help:"Set true and save to rotate signs now (rewrite local STRM), then auto reset to false"`
	SaveStrmToLocal   bool   `json:"SaveStrmToLocal" default:"false" help:"Save generated files to local disk"`
	SaveStrmLocalPath string `json:"SaveStrmLocalPath" type:"text" help:"Local path for generated files"`
	SaveLocalMode     string `json:"SaveLocalMode" type:"select" help:"Local save mode" options:"insert,update,sync" default:"insert"`
	Version           int
}

var config = driver.Config{
	Name:        "Strm",
	LocalSort:   true,
	OnlyProxy:   true,
	NoCache:     true,
	NoUpload:    true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Strm{Addition: Addition{EncodePath: true}}
	})
}
