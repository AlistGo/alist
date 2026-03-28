package streamtape

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootID
	APILogin           string `json:"api_login" required:"true" help:"API Login from Streamtape account settings"`
	APIKey             string `json:"api_key" required:"true" help:"API Key from Streamtape account settings"`
	RangeMode          string `json:"range_mode" type:"select" options:"chunk,full,percent" default:"chunk" help:"Range strategy for preview: chunk=bounded ranges, full=single full-tail range, percent=part size by file percentage"`
	RangeChunkMB       int    `json:"range_chunk_mb" type:"number" default:"8" help:"Chunk mode part size in MB"`
	RangeConcurrency   int    `json:"range_concurrency" type:"number" default:"4" help:"Chunk mode concurrent upstream requests"`
	RangePercent       int    `json:"range_percent" type:"number" default:"15" help:"Percent mode part size percentage (1-100)"`
	EnableRangeControl bool   `json:"enable_range_control" default:"true" help:"Enable driver-level range shaping for smoother streaming"`
	Sha256             string `json:"sha256" help:"Expected SHA256 hash for upload verification (optional)"`
}

var config = driver.Config{
	Name:              "Streamtape",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         true,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "0",
	CheckStatus:       false,
	Alert:             "warning|Moving files to root folder is not supported by Streamtape API",
	NoOverwriteUpload: false,
	ProxyRangeOption:  true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Streamtape{}
	})
}
