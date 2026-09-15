package _123Open

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

// Authentication modes.
const (
	// AuthClientCredentials obtains the access token from client_id/client_secret,
	// the path available to personal developers.
	AuthClientCredentials = "client_credentials"
	// AuthToken uses an access_token (plus optional refresh_token) issued through
	// an already-onboarded enterprise's OAuth application.
	AuthToken = "token"
)

type Addition struct {
	driver.RootID

	AuthMode string `json:"auth_mode" type:"select" options:"token,client_credentials" default:"token" help:"token: use the access_token/refresh_token, no developer application needed. client_credentials: use the clientID/clientSecret of your own application on the open platform."`

	// token mode.
	AccessToken string `json:"access_token" required:"true" show_when:"auth_mode=token" help:"Get it at https://alistgo.com/zh/tool/123pan/request.html"`
	// RefreshToken is single-use: every refresh returns a new one, which is
	// written back to the storage config.
	RefreshToken string `json:"refresh_token" show_when:"auth_mode=token" help:"Obtained together with the access_token. Single-use and rotated on every refresh; the new value is saved back automatically."`

	// client_credentials mode, also used to refresh tokens in token mode.
	ClientID     string `json:"client_id" label:"clientID" required:"true" show_when:"auth_mode=client_credentials"`
	ClientSecret string `json:"client_secret" label:"clientSecret" required:"true" show_when:"auth_mode=client_credentials"`

	// Direct link signing (anti-leech), configured in the open platform console.
	PrivateKey    string `json:"private_key" help:"Direct link signing key. Leave empty to return unsigned links."`
	UID           uint64 `json:"uid" type:"number" help:"User ID used by direct link signing."`
	ValidDuration int64  `json:"valid_duration" type:"number" default:"30" help:"Validity of a signed direct link, in minutes."`

	UseDirectLink bool `json:"use_direct_link" help:"Resolve downloads through the direct link service instead of the download API. Requires developer benefits and an enabled direct link folder."`

	UploadThread int `json:"upload_thread" type:"number" default:"3" help:"Concurrent slice uploads."`
}

var config = driver.Config{
	Name:        "123 Open",
	LocalSort:   false,
	DefaultRoot: "0",
	CheckStatus: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Open123{}
	})
}
