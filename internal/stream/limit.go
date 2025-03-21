package stream

import (
	"context"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"golang.org/x/time/rate"
	"io"
	"time"
)

type Limiter interface {
	Limit() rate.Limit
	Burst() int
	TokensAt(time.Time) float64
	Tokens() float64
	Allow() bool
	AllowN(time.Time, int) bool
	Reserve() *rate.Reservation
	ReserveN(time.Time, int) *rate.Reservation
	Wait(context.Context) error
	WaitN(context.Context, int) error
	SetLimit(rate.Limit)
	SetLimitAt(time.Time, rate.Limit)
	SetBurst(int)
	SetBurstAt(time.Time, int)
}

var (
	ClientDownloadLimit Limiter
	ClientUploadLimit   Limiter
	ServerDownloadLimit Limiter
	ServerUploadLimit   Limiter
)

type RateLimitReader struct {
	io.Reader
	Limiter Limiter
	Ctx     context.Context
}

func (r *RateLimitReader) Read(p []byte) (n int, err error) {
	if r.Ctx != nil && utils.IsCanceled(r.Ctx) {
		return 0, r.Ctx.Err()
	}
	n, err = r.Reader.Read(p)
	if err != nil {
		return
	}
	if r.Limiter != nil {
		if r.Ctx == nil {
			r.Ctx = context.Background()
		}
		err = r.Limiter.WaitN(r.Ctx, n)
	}
	return
}

func (r *RateLimitReader) Close() error {
	if c, ok := r.Reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type RateLimitWriter struct {
	io.Writer
	Limiter Limiter
	Ctx     context.Context
}

func (w *RateLimitWriter) Write(p []byte) (n int, err error) {
	if w.Ctx != nil && utils.IsCanceled(w.Ctx) {
		return 0, w.Ctx.Err()
	}
	n, err = w.Writer.Write(p)
	if err != nil {
		return
	}
	if w.Limiter != nil {
		if w.Ctx == nil {
			w.Ctx = context.Background()
		}
		err = w.Limiter.WaitN(w.Ctx, n)
	}
	return
}

func (w *RateLimitWriter) Close() error {
	if c, ok := w.Writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type RateLimitFile struct {
	model.File
	Limiter Limiter
	Ctx     context.Context
}

func (r *RateLimitFile) Read(p []byte) (n int, err error) {
	if r.Ctx != nil && utils.IsCanceled(r.Ctx) {
		return 0, r.Ctx.Err()
	}
	n, err = r.File.Read(p)
	if err != nil {
		return
	}
	if r.Limiter != nil {
		if r.Ctx == nil {
			r.Ctx = context.Background()
		}
		err = r.Limiter.WaitN(r.Ctx, n)
	}
	return
}

func (r *RateLimitFile) ReadAt(p []byte, off int64) (n int, err error) {
	if r.Ctx != nil && utils.IsCanceled(r.Ctx) {
		return 0, r.Ctx.Err()
	}
	n, err = r.File.ReadAt(p, off)
	if err != nil {
		return
	}
	if r.Limiter != nil {
		if r.Ctx == nil {
			r.Ctx = context.Background()
		}
		err = r.Limiter.WaitN(r.Ctx, n)
	}
	return
}

type RateLimitRangeReadCloser struct {
	model.RangeReadCloserIF
	Limiter Limiter
}

func (rrc *RateLimitRangeReadCloser) RangeRead(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
	rc, err := rrc.RangeReadCloserIF.RangeRead(ctx, httpRange)
	if err != nil {
		return nil, err
	}
	return &RateLimitReader{
		Reader:  rc,
		Limiter: rrc.Limiter,
		Ctx:     ctx,
	}, nil
}
