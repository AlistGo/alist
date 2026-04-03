package chunker

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

const (
	defaultChunkSize    int64 = 2147483648
	defaultChunkNameFmt       = "{name}.rclone_chunk.{chunk:3}"
	defaultMetaFormat         = "simplejson"
	defaultHashType           = "md5"
	defaultStartFrom          = 1
)

type Addition struct {
	RemotePath           string `json:"remote_path" required:"true" help:"Primary AList mounted folder path used to store metadata and small files, e.g. /my-storage/chunks"`
	RemotePaths          string `json:"remote_paths" type:"text" help:"Additional AList mounted folder paths, one per line. Chunk files will be distributed across remote_path and these extra paths."`
	StoreChunksInPrimary bool   `json:"store_chunks_in_primary" type:"bool" default:"true" help:"When extra remote paths are configured, also store chunk files in remote_path"`
	ChunkSize            int64  `json:"chunk_size" type:"number" required:"true" default:"2147483648" help:"Files larger than this will be split into chunks"`
	NameFormat           string `json:"name_format" required:"true" default:"{name}.rclone_chunk.{chunk:3}" help:"Magic tokens: {name}, {chunk}, {chunk:N}. Name token must appear before chunk token."`
	StartFrom            int    `json:"start_from" type:"number" required:"true" default:"1" help:"Chunk number base, usually 0 or 1"`
	MetaFormat           string `json:"meta_format" type:"select" required:"true" options:"simplejson,none" default:"simplejson" help:"simplejson is compatible with rclone chunker metadata"`
	HashType             string `json:"hash_type" type:"select" required:"true" options:"none,md5,sha1" default:"md5" help:"Hash stored in metadata for chunked files"`
}

var config = driver.Config{
	Name:              "Chunker",
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
		return &Chunker{}
	})
}
