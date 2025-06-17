package base

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/net"
	"github.com/go-resty/resty/v2"
)

var (
	NoRedirectClient *resty.Client
	RestyClient      *resty.Client
	HttpClient       *http.Client
)
var UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36"
var DefaultTimeout = time.Second * 30

func InitClient() {
	NoRedirectClient = resty.New().SetRedirectPolicy(
		resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}),
	).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify})
	NoRedirectClient.SetHeader("user-agent", UserAgent)

	RestyClient = NewRestyClient()
	HttpClient = net.NewHttpClient()
}

func NewRestyClient() *resty.Client {
	client := resty.New().
		SetHeader("user-agent", UserAgent).
		SetRetryCount(3).
		SetRetryResetReaders(true).
		SetTimeout(DefaultTimeout).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify})
	return client
}
