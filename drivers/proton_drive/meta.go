package protondrive

/*
Package protondrive
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-18

Thanks to @henrybear327 for modded go-proton-api & Proton-API-Bridge

The power of open-source, the force of teamwork and the magic of reverse engineering!


D@' 3z K!7 - The King Of Cracking

Да здравствует Родина))
*/

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	//driver.RootID

	Username  string `json:"username" required:"true" type:"string"`
	Password  string `json:"password" required:"true" type:"string"`
	TwoFACode string `json:"two_fa_code,omitempty" type:"string"`
}

type Config struct {
	Name        string `json:"name"`
	LocalSort   bool   `json:"local_sort"`
	OnlyLocal   bool   `json:"only_local"`
	OnlyProxy   bool   `json:"only_proxy"`
	NoCache     bool   `json:"no_cache"`
	NoUpload    bool   `json:"no_upload"`
	NeedMs      bool   `json:"need_ms"`
	DefaultRoot string `json:"default_root"`
}

var config = driver.Config{
	Name:              "ProtonDrive",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "/",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &ProtonDrive{
			apiBase:             "https://drive.proton.me/api",
			appVersion:          "windows-drive@1.11.3+rclone+proton",
			credentialCacheFile: ".prtcrd",
			protonJson:          "application/vnd.protonmail.v1+json",
			sdkVersion:          "js@0.3.0",
			userAgent:           "ProtonDrive/v1.70.0 (Windows NT 10.0.22000; Win64; x64)",
			webDriveAV:          "web-drive@5.2.0+0f69f7a8",
		}
	})
}
