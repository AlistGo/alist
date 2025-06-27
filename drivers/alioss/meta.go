package alioss

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	Endpoint        string `json:"endpoint" required:"true" help:"OSS endpoint, e.g. https://oss-cn-hangzhou.aliyuncs.com"`
	AccessKeyId     string `json:"access_key_id" required:"true" help:"OSS Access Key ID"`
	AccessKeySecret string `json:"access_key_secret" required:"true" help:"OSS Access Key Secret"`
	BucketName      string `json:"bucket_name" required:"true" help:"OSS Bucket Name"`
	SignURLExpire   int    `json:"sign_url_expire" type:"number" default:"4" help:"Sign URL expire time in hours"`
	Placeholder     string `json:"placeholder" help:"Placeholder file name for empty folders"`
}

var config = driver.Config{
	Name:        "ALiOSS",
	DefaultRoot: "/",
	LocalSort:   true,
	CheckStatus: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &AliOSS{}
	})
}
