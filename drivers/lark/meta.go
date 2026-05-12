package lark

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	driver.RootPath
	// define other
	AppId                    string `json:"app_id" type:"text" help:"app id"`
	AppSecret                string `json:"app_secret" type:"text" help:"app secret"`
	UserAccessToken          string `json:"user_access_token" type:"text" help:"optional cached user access token for personal drive access"`
	RefreshToken             string `json:"refresh_token" type:"text" help:"optional refresh token for user access token auto refresh"`
	UserAccessTokenExpiresAt int64  `json:"user_access_token_expires_at" type:"number" help:"user access token expires at unix timestamp"`
	RefreshTokenExpiresAt    int64  `json:"refresh_token_expires_at" type:"number" help:"refresh token expires at unix timestamp"`
	ExternalMode             bool   `json:"external_mode" type:"bool" help:"external mode"`
	TenantUrlPrefix          string `json:"tenant_url_prefix" type:"text" help:"tenant url prefix"`
}

var config = driver.Config{
	Name:              "Lark",
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
		return &Lark{}
	})
}
