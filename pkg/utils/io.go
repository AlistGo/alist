package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/exp/constraints"

	log "github.com/sirupsen/logrus"
)

// here is some syntaxic sugar inspired by the Tomas Senart's video,
// it allows me to inline the Reader interface
type readerFunc func(p []byte) (n int, err error)

func (rf readerFunc) Read(p []byte) (n int, err error) { return rf(p) }

// CopyWithCtx slightly modified function signature:
// - context has been added in order to propagate cancellation
// - I do not return the number of bytes written, has it is not useful in my use case
func CopyWithCtx(ctx context.Context, out io.Writer, in io.Reader, size int64, progress func(percentage float64)) error {
	// Copy will call the Reader and Writer interface multiple time, in order
	// to copy by chunk (avoiding loading the whole file in memory).
	// I insert the ability to cancel before read time as it is the earliest
	// possible in the call process.
	var finish int64 = 0
	s := size / 100
	_, err := CopyWithBuffer(out, readerFunc(func(p []byte) (int, error) {
		// golang non-blocking channel: https://gobyexample.com/non-blocking-channel-operations
		select {
		// if context has been canceled
		case <-ctx.Done():
			// stop process and propagate "context canceled" error
			return 0, ctx.Err()
		default:
			// otherwise just run default io.Reader implementation
			n, err := in.Read(p)
			if s > 0 && (err == nil || err == io.EOF) {
				finish += int64(n)
				progress(float64(finish) / float64(s))
			}
			return n, err
		}
	}))
	return err
}

type limitWriter struct {
	w     io.Writer
	limit int64
}

func (l *limitWriter) Write(p []byte) (n int, err error) {
	lp := len(p)
	if l.limit > 0 {
		if int64(lp) > l.limit {
			p = p[:l.limit]
		}
		l.limit -= int64(len(p))
		_, err = l.w.Write(p)
	}
	return lp, err
}

func LimitWriter(w io.Writer, limit int64) io.Writer {
	return &limitWriter{w: w, limit: limit}
}

type ReadCloser struct {
	io.Reader
	io.Closer
}

type CloseFunc func() error

func (c CloseFunc) Close() error {
	return c()
}

func NewReadCloser(reader io.Reader, close CloseFunc) io.ReadCloser {
	return ReadCloser{
		Reader: reader,
		Closer: close,
	}
}

func NewLimitReadCloser(reader io.Reader, close CloseFunc, limit int64) io.ReadCloser {
	return NewReadCloser(io.LimitReader(reader, limit), close)
}

type MultiReadable struct {
	originReader io.Reader
	reader       io.Reader
	cache        *bytes.Buffer
}

func NewMultiReadable(reader io.Reader) *MultiReadable {
	return &MultiReadable{
		originReader: reader,
		reader:       reader,
	}
}

func (mr *MultiReadable) Read(p []byte) (int, error) {
	n, err := mr.reader.Read(p)
	if _, ok := mr.reader.(io.Seeker); !ok && n > 0 {
		if mr.cache == nil {
			mr.cache = &bytes.Buffer{}
		}
		mr.cache.Write(p[:n])
	}
	return n, err
}

func (mr *MultiReadable) Reset() error {
	if seeker, ok := mr.reader.(io.Seeker); ok {
		_, err := seeker.Seek(0, io.SeekStart)
		return err
	}
	if mr.cache != nil && mr.cache.Len() > 0 {
		mr.reader = io.MultiReader(mr.cache, mr.reader)
		mr.cache = nil
	}
	return nil
}

func (mr *MultiReadable) Close() error {
	if closer, ok := mr.originReader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func Retry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		//fmt.Println("This is attempt number", i)
		if i > 0 {
			log.Println("retrying after error:", err)
			time.Sleep(sleep)
			sleep *= 2
		}
		err = f()
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

type ClosersIF interface {
	io.Closer
	Add(closer io.Closer)
	AddClosers(closers Closers)
	GetClosers() Closers
}

type Closers struct {
	closers []io.Closer
}

func (c *Closers) GetClosers() Closers {
	return *c
}

var _ ClosersIF = (*Closers)(nil)

func (c *Closers) Close() error {
	var errs []error
	for _, closer := range c.closers {
		if closer != nil {
			errs = append(errs, closer.Close())
		}
	}
	return errors.Join(errs...)
}
func (c *Closers) Add(closer io.Closer) {
	c.closers = append(c.closers, closer)

}
func (c *Closers) AddClosers(closers Closers) {
	c.closers = append(c.closers, closers.closers...)
}

func EmptyClosers() Closers {
	return Closers{[]io.Closer{}}
}
func NewClosers(c ...io.Closer) Closers {
	return Closers{c}
}

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
func Max[T constraints.Ordered](a, b T) T {
	if a < b {
		return b
	}
	return a
}

var IoBuffPool = &sync.Pool{
	New: func() interface{} {
		return make([]byte, 32*1024*2) // Two times of size in io package
	},
}

func CopyWithBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	buff := IoBuffPool.Get().([]byte)
	defer IoBuffPool.Put(buff)
	written, err = io.CopyBuffer(dst, src, buff)
	if err != nil {
		return
	}
	return written, nil
}

func CopyWithBufferN(dst io.Writer, src io.Reader, n int64) (written int64, err error) {
	written, err = CopyWithBuffer(dst, io.LimitReader(src, n))
	if written == n {
		return n, nil
	}
	if written < n && err == nil {
		// src stopped early; must have been EOF.
		err = io.EOF
	}
	return
}
