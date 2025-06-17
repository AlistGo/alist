package model

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type ListArgs struct {
	ReqPath           string
	S3ShowPlaceholder bool
	Refresh           bool
}

type LinkArgs struct {
	IP       string
	Header   http.Header
	Type     string
	HttpReq  *http.Request
	Redirect bool
}

type Link struct {
	URL             string            `json:"url"`    // most common way
	Header          http.Header       `json:"header"` // needed header (for url)
	RangeReadCloser RangeReadCloserIF `json:"-"`      // recommended way if can't use URL
	MFile           File              `json:"-"`      // best for local,smb... file system, which exposes MFile

	Expiration *time.Duration // local cache expire Duration
	IPCacheKey bool           `json:"-"` // add ip to cache key

	//for accelerating request, use multi-thread downloading
	Concurrency int `json:"concurrency"`
	PartSize    int `json:"part_size"`
}

type OtherArgs struct {
	Obj    Obj
	Method string
	Data   interface{}
}

type FsOtherArgs struct {
	Path   string      `json:"path" form:"path"`
	Method string      `json:"method" form:"method"`
	Data   interface{} `json:"data" form:"data"`
}

type ArchiveArgs struct {
	Password string
	LinkArgs
}

type ArchiveInnerArgs struct {
	ArchiveArgs
	InnerPath string
}

type ArchiveMetaArgs struct {
	ArchiveArgs
	Refresh bool
}

type ArchiveListArgs struct {
	ArchiveInnerArgs
	Refresh bool
}

type ArchiveDecompressArgs struct {
	ArchiveInnerArgs
	CacheFull     bool
	PutIntoNewDir bool
}

type RangeReadCloserIF interface {
	RangeRead(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error)
	utils.ClosersIF
}

var _ RangeReadCloserIF = (*RangeReadCloser)(nil)

type RangeReadCloser struct {
	RangeReader RangeReaderFunc
	utils.Closers
}

func (r *RangeReadCloser) RangeRead(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
	rc, err := r.RangeReader(ctx, httpRange)
	r.Closers.Add(rc)
	return rc, err
}

// type WriterFunc func(w io.Writer) error
type RangeReaderFunc func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error)
