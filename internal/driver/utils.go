package driver

import (
	"context"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"io"
)

type UpdateProgress = model.UpdateProgress

type Progress struct {
	Total int64
	Done  int64
	up    UpdateProgress
}

func (p *Progress) Write(b []byte) (n int, err error) {
	n = len(b)
	p.Done += int64(n)
	p.up(float64(p.Done) / float64(p.Total) * 100)
	return
}

func NewProgress(total int64, up UpdateProgress) *Progress {
	return &Progress{
		Total: total,
		up:    up,
	}
}

type RateLimitReader = stream.RateLimitReader

type RateLimitWriter = stream.RateLimitWriter

type RateLimitFile = stream.RateLimitFile

func NewLimitedUploadStream(ctx context.Context, r io.Reader) *RateLimitReader {
	return &RateLimitReader{
		Reader:  r,
		Limiter: stream.ServerUploadLimit,
		Ctx:     ctx,
	}
}

func NewLimitedUploadFile(ctx context.Context, f model.File) *RateLimitFile {
	return &RateLimitFile{
		File:    f,
		Limiter: stream.ServerUploadLimit,
		Ctx:     ctx,
	}
}

func ServerUploadLimitWaitN(ctx context.Context, n int) error {
	return stream.ServerUploadLimit.WaitN(ctx, n)
}

type ReaderWithCtx = stream.ReaderWithCtx

type ReaderUpdatingProgress = stream.ReaderUpdatingProgress

type SimpleReaderWithSize = stream.SimpleReaderWithSize
