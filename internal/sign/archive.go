package sign

import (
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/sign"
)

var onceArchive sync.Once
var instanceArchive sign.Sign

func SignArchive(data string) string {
	expire := setting.GetInt(conf.LinkExpiration, 0)
	if expire == 0 {
		return NotExpiredArchive(data)
	} else {
		return WithDurationArchive(data, time.Duration(expire)*time.Hour)
	}
}

func WithDurationArchive(data string, d time.Duration) string {
	onceArchive.Do(InstanceArchive)
	return instanceArchive.Sign(data, time.Now().Add(d).Unix())
}

func NotExpiredArchive(data string) string {
	onceArchive.Do(InstanceArchive)
	return instanceArchive.Sign(data, 0)
}

func VerifyArchive(data string, sign string) error {
	onceArchive.Do(InstanceArchive)
	return instanceArchive.Verify(data, sign)
}

func InstanceArchive() {
	instanceArchive = sign.NewHMACSign([]byte(setting.GetStr(conf.Token) + "-archive"))
}
