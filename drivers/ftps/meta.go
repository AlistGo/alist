package ftps

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/axgle/mahonia"
)

func encode(str string, encoding string) string {
	if encoding == "" {
		return str
	}
	encoder := mahonia.NewEncoder(encoding)
	return encoder.ConvertString(str)
}

func decode(str string, encoding string) string {
	if encoding == "" {
		return str
	}
	decoder := mahonia.NewDecoder(encoding)
	return decoder.ConvertString(str)
}

type Addition struct {
	Address               string `json:"address" required:"true"`
	Encoding              string `json:"encoding" required:"false"`
	Username              string `json:"username" required:"true"`
	Password              string `json:"password" required:"true"`
	TLSMode               string `json:"tls_mode" type:"select" options:"Explicit,Implicit" default:"Explicit" required:"true" help:"Explicit: STARTTLS on port 21; Implicit: direct TLS on port 990"`
	TLSInsecureSkipVerify bool   `json:"tls_insecure_skip_verify" default:"false" help:"Allow insecure TLS connections (e.g. self-signed certificates)"`
	driver.RootPath
}

var config = driver.Config{
	Name:        "FTPS",
	LocalSort:   true,
	OnlyLocal:   true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &FTPS{}
	})
}
