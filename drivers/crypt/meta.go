package crypt

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	// Usually one of two
	//driver.RootPath
	//driver.RootID
	// define other

	FileNameEnc string `json:"filename_encryption" type:"select" required:"true" options:"off,standard,obfuscate" default:"off"`
	DirNameEnc  string `json:"directory_name_encryption" type:"select" required:"true" options:"false,true" default:"false"`
	RemotePath  string `json:"remote_path" required:"true" help:"AList mounted folder path used to store encrypted data, e.g. /my-storage/secret"`

	Password         string `json:"password" required:"true" confidential:"true" help:"the main password"`
	Salt             string `json:"salt" confidential:"true"  help:"If you don't know what is salt, treat it as a second password. Optional but recommended"`
	EncryptedSuffix  string `json:"encrypted_suffix" required:"true" default:".bin" help:"for advanced user only! encrypted files will have this suffix"`
	FileNameEncoding string `json:"filename_encoding" type:"select" required:"true" options:"base64,base32,base32768" default:"base64" help:"for advanced user only!"`

	Thumbnail bool `json:"thumbnail" required:"true" default:"false" help:"enable thumbnail which pre-generated under .thumbnails folder"`

	ShowHidden bool `json:"show_hidden"  default:"true" required:"false" help:"show hidden directories and files"`
}

var config = driver.Config{
	Name:              "Crypt",
	LocalSort:         true,
	OnlyLocal:         false,
	OnlyProxy:         true,
	NoCache:           true,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "/",
	CheckStatus:       false,
	Alert:             "",
	NoOverwriteUpload: false,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Crypt{}
	})
}
