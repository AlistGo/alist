package _115

import (
	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/alist-org/alist/v3/drivers/base"
	log "github.com/sirupsen/logrus"
)

var (
	md5Salt = "Qclm8MGWUv59TnrR0XPg"
	appVer  = "27.0.5.7"
)

func (d *Pan115) getAppVersion() ([]driver115.AppVersion, error) {
	result := driver115.VersionResp{}
	resp, err := base.RestyClient.R().Get(driver115.ApiGetVersion)

	err = driver115.CheckErr(err, &result, resp)
	if err != nil {
		return nil, err
	}

	return result.Data.GetAppVersions(), nil
}

func (d *Pan115) getAppVer() string {
	// todo add some cache？
	vers, err := d.getAppVersion()
	if err != nil {
		log.Warnf("[115] get app version failed: %v", err)
		return appVer
	}
	for _, ver := range vers {
		if ver.AppName == "win" {
			return ver.Version
		}
	}
	return appVer
}

func (d *Pan115) initAppVer() {
	appVer = d.getAppVer()
}
