package chunker

import (
	"regexp"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

const (
	ctrlTypeRegStr        = `[a-z][a-z0-9]{2,6}`
	tempSuffixFormat      = `_%04s`
	tempSuffixRegStr      = `_([0-9a-z]{4,9})`
	tempSuffixRegOld      = `\.\.tmp_([0-9]{10,13})`
	maxMetadataSizeRead   = 1023
	maxMetadataSizeWrite  = 255
	maxSafeChunkNumber    = 10000000
	chunkerMetadataVerion = 2
)

var ctrlTypeRegexp = regexp.MustCompile(`^` + ctrlTypeRegStr + `$`)
var chunkTokenRegexp = regexp.MustCompile(`\{chunk(?::([0-9]+))?\}`)

type Chunker struct {
	model.Storage
	Addition
	remoteStorage driver.Driver
	remoteTargets []remoteTarget
	dataNameFmt   string
	nameRegexp    *regexp.Regexp
}

type remoteTarget struct {
	MountPath string
	Storage   driver.Driver
}

type locatedObj struct {
	Obj         model.Obj
	RemoteIndex int
}

type objectLocation struct {
	LogicalPath string
	RemoteIndex int
}

type metadataJSON struct {
	Version  *int   `json:"ver"`
	Size     *int64 `json:"size"`
	ChunkNum *int   `json:"nchunks"`
	MD5      string `json:"md5,omitempty"`
	SHA1     string `json:"sha1,omitempty"`
	XactID   string `json:"txn,omitempty"`
}

type chunkMetadata struct {
	Version int
	Size    int64
	NChunks int
	MD5     string
	SHA1    string
	XactID  string
}

type chunkPart struct {
	No          int
	Size        int64
	XactID      string
	RemoteIndex int
}

type groupInfo struct {
	base        *locatedObj
	partsByXact map[string]map[int]chunkPart
}

type Object struct {
	model.Object
	Main            model.Obj
	MainRemoteIndex int
	Parts           []chunkPart
	Meta            *chunkMetadata
	Chunked         bool
	UsesMeta        bool
}

type linkedPart struct {
	part chunkPart
	link *model.Link
}
