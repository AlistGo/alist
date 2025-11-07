package _189pc

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	Username   string `json:"username" required:"true"`
	Password   string `json:"password" required:"true"`
	VCode      string `json:"validate_code"`
	StripEmoji bool   `json:"strip_emoji" help:"Remove four-byte characters (e.g., emoji) before upload"`
	driver.RootID
	OrderBy        string `json:"order_by" type:"select" options:"filename,filesize,lastOpTime" default:"filename"`
	OrderDirection string `json:"order_direction" type:"select" options:"asc,desc" default:"asc"`
	Type           string `json:"type" type:"select" options:"personal,family" default:"personal"`
	FamilyID       string `json:"family_id"`
	UploadMethod   string `json:"upload_method" type:"select" options:"stream,rapid,old" default:"stream"`
	UploadThread   string `json:"upload_thread" default:"3" help:"1<=thread<=32"`
	FamilyTransfer bool   `json:"family_transfer"`
	RapidUpload    bool   `json:"rapid_upload"`
	NoUseOcr       bool   `json:"no_use_ocr"`
}

var config = driver.Config{
	Name:        "189CloudPC",
	DefaultRoot: "-11",
	CheckStatus: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Cloud189PC{}
	})
}
