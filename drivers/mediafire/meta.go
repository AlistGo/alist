package mediafire

/*
Package mediafire
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-11

D@' 3z K!7 - The King Of Cracking
*/

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	//driver.RootID

	SessionToken string `json:"session_token" required:"true" type:"string" help:"Required for MediaFire API"`
	Cookie       string `json:"cookie" required:"true" type:"string" help:"Required for navigation"`

	OrderBy        string `json:"order_by" type:"select" options:"name,time,size" default:"name"`
	OrderDirection string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	ChunkSize      int64  `json:"chunk_size" type:"number" default:"100"`
}

var config = driver.Config{
	Name:              "MediaFire",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "/",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Mediafire{
			appBase:         "https://app.mediafire.com",
			apiBase:         "https://www.mediafire.com/api/1.5",
			hostBase:        "https://www.mediafire.com",
			maxRetries:      3,
			secChUa:         "\"Not)A;Brand\";v=\"8\", \"Chromium\";v=\"139\", \"Google Chrome\";v=\"139\"",
			secChUaPlatform: "Windows",
			userAgent:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36",
		}
	})
}
