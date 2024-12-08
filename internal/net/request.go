package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/pkg/utils"

	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	log "github.com/sirupsen/logrus"
)

// DefaultDownloadPartSize is the default range of bytes to get at a time when
// using Download().
const DefaultDownloadPartSize = 1024 * 1024 * 10

// DefaultDownloadConcurrency is the default number of goroutines to spin up
// when using Download().
const DefaultDownloadConcurrency = 2

// DefaultPartBodyMaxRetries is the default number of retries to make when a part fails to download.
const DefaultPartBodyMaxRetries = 3

type Downloader struct {
	PartSize int

	// PartBodyMaxRetries is the number of retry attempts to make for failed part downloads.
	PartBodyMaxRetries int

	// The number of goroutines to spin up in parallel when sending parts.
	// If this is set to zero, the DefaultDownloadConcurrency value will be used.
	//
	// Concurrency of 1 will download the parts sequentially.
	Concurrency int

	//RequestParam        HttpRequestParams
	HttpClient HttpRequestFunc
}
type HttpRequestFunc func(ctx context.Context, params *HttpRequestParams) (*http.Response, error)

func NewDownloader(options ...func(*Downloader)) *Downloader {
	d := &Downloader{
		HttpClient:         DefaultHttpRequestFunc,
		PartSize:           DefaultDownloadPartSize,
		PartBodyMaxRetries: DefaultPartBodyMaxRetries,
		Concurrency:        DefaultDownloadConcurrency,
	}
	for _, option := range options {
		option(d)
	}
	return d
}

// Download The Downloader makes multi-thread http requests to remote URL, each chunk(except last one) has PartSize,
// cache some data, then return Reader with assembled data
// Supports range, do not support unknown FileSize, and will fail if FileSize is incorrect
// memory usage is at about Concurrency*PartSize, use this wisely
func (d Downloader) Download(ctx context.Context, p *HttpRequestParams) (readCloser io.ReadCloser, err error) {

	var finalP HttpRequestParams
	awsutil.Copy(&finalP, p)
	if finalP.Range.Length == -1 {
		finalP.Range.Length = finalP.Size - finalP.Range.Start
	}
	impl := downloader{params: &finalP, cfg: d, ctx: ctx}

	// Ensures we don't need nil checks later on

	impl.partBodyMaxRetries = d.PartBodyMaxRetries

	if impl.cfg.Concurrency == 0 {
		impl.cfg.Concurrency = DefaultDownloadConcurrency
	}

	if impl.cfg.PartSize == 0 {
		impl.cfg.PartSize = DefaultDownloadPartSize
	}

	return impl.download()
}

// downloader is the implementation structure used internally by Downloader.
type downloader struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Downloader

	params       *HttpRequestParams //http request params
	chunkChannel chan chunk         //chunk chanel

	//wg sync.WaitGroup
	m sync.Mutex

	nextChunk int //next chunk id
	chunks    []chunk
	bufs      []*Buf
	//totalBytes int64
	written int64 //total bytes of file downloaded from remote
	err     error

	partBodyMaxRetries int
}

// download performs the implementation of the object download across ranged GETs.
func (d *downloader) download() (io.ReadCloser, error) {
	d.ctx, d.cancel = context.WithCancel(d.ctx)

	pos := d.params.Range.Start
	maxPos := d.params.Range.Start + d.params.Range.Length
	id := 0
	for pos < maxPos {
		finalSize := int64(d.cfg.PartSize)
		//check boundary
		if pos+finalSize > maxPos {
			finalSize = maxPos - pos
		}
		c := chunk{start: pos, size: finalSize, id: id}
		d.chunks = append(d.chunks, c)
		pos += finalSize
		id++
	}
	if len(d.chunks) < d.cfg.Concurrency {
		d.cfg.Concurrency = len(d.chunks)
	}

	if d.cfg.Concurrency == 1 {
		resp, err := d.cfg.HttpClient(d.ctx, d.params)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}

	// workers
	d.chunkChannel = make(chan chunk, d.cfg.Concurrency)

	for i := 0; i < d.cfg.Concurrency; i++ {
		buf := NewBuf(d.ctx, d.cfg.PartSize, i)
		d.bufs = append(d.bufs, buf)
		go d.downloadPart()
	}
	// initial tasks
	for i := 0; i < d.cfg.Concurrency; i++ {
		d.sendChunkTask()
	}

	var rc io.ReadCloser = NewMultiReadCloser(d.chunks[0].buf, d.interrupt, d.finishBuf)

	// Return error
	return rc, d.err
}
func (d *downloader) sendChunkTask() *chunk {
	ch := &d.chunks[d.nextChunk]
	ch.buf = d.getBuf(d.nextChunk)
	ch.buf.Reset(int(ch.size))
	d.chunkChannel <- *ch
	d.nextChunk++
	return ch
}

// when the final reader Close, we interrupt
func (d *downloader) interrupt() error {

	d.cancel()
	if d.written != d.params.Range.Length {
		log.Debugf("Downloader interrupt before finish")
		if d.getErr() == nil {
			d.setErr(fmt.Errorf("interrupted"))
		}
	}
	defer func() {
		close(d.chunkChannel)
		for _, buf := range d.bufs {
			buf.Close()
		}
	}()
	return d.err
}
func (d *downloader) getBuf(id int) (b *Buf) {

	return d.bufs[id%d.cfg.Concurrency]
}
func (d *downloader) finishBuf(id int) (isLast bool, buf *Buf) {
	if id >= len(d.chunks)-1 {
		return true, nil
	}
	if d.nextChunk > id+1 {
		return false, d.getBuf(id + 1)
	}
	ch := d.sendChunkTask()
	return false, ch.buf
}

// downloadPart is an individual goroutine worker reading from the ch channel
// and performing Http request on the data with a given byte range.
func (d *downloader) downloadPart() {
	//defer d.wg.Done()
	for {
		c, ok := <-d.chunkChannel
		if !ok {
			break
		}
		if d.getErr() != nil {
			// Drain the channel if there is an error, to prevent deadlocking
			// of download producer.
			continue
		}
		log.Debugf("downloadPart tried to get chunk")
		if err := d.downloadChunk(&c); err != nil {
			d.setErr(err)
		}
	}
}

// downloadChunk downloads the chunk
func (d *downloader) downloadChunk(ch *chunk) error {
	log.Debugf("start new chunk %+v buffer_id =%d", ch, ch.id)
	var n int64
	var err error
	params := d.getParamsFromChunk(ch)
	for retry := 0; retry <= d.partBodyMaxRetries; retry++ {
		if d.getErr() != nil {
			return d.getErr()
		}
		n, err = d.tryDownloadChunk(params, ch)
		if err == nil {
			break
		}
		// Check if the returned error is an errReadingBody.
		// If err is errReadingBody this indicates that an error
		// occurred while copying the http response body.
		// If this occurs we unwrap the err to set the underlying error
		// and attempt any remaining retries.
		if bodyErr, ok := err.(*errReadingBody); ok {
			err = bodyErr.Unwrap()
		} else {
			return err
		}

		//ch.cur = 0

		log.Debugf("object part body download interrupted %s, err, %v, retrying attempt %d",
			params.URL, err, retry)
	}

	d.incrWritten(n)
	log.Debugf("down_%d downloaded chunk", ch.id)
	//ch.buf.buffer.wg1.Wait()
	//log.Debugf("down_%d downloaded chunk,wg wait passed", ch.id)
	return err
}

func (d *downloader) tryDownloadChunk(params *HttpRequestParams, ch *chunk) (int64, error) {

	resp, err := d.cfg.HttpClient(d.ctx, params)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	//only check file size on the first task
	if ch.id == 0 {
		err = d.checkTotalBytes(resp)
		if err != nil {
			return 0, err
		}
	}

	n, err := utils.CopyWithBuffer(ch.buf, resp.Body)

	if err != nil {
		return n, &errReadingBody{err: err}
	}
	if n != ch.size {
		err = fmt.Errorf("chunk download size incorrect, expected=%d, got=%d", ch.size, n)
		return n, &errReadingBody{err: err}
	}

	return n, nil
}
func (d *downloader) getParamsFromChunk(ch *chunk) *HttpRequestParams {
	var params HttpRequestParams
	awsutil.Copy(&params, d.params)

	// Get the getBuf byte range of data
	params.Range = http_range.Range{Start: ch.start, Length: ch.size}
	return &params
}

func (d *downloader) checkTotalBytes(resp *http.Response) error {
	var err error
	var totalBytes int64 = math.MinInt64
	contentRange := resp.Header.Get("Content-Range")
	if len(contentRange) == 0 {
		// ContentRange is nil when the full file contents is provided, and
		// is not chunked. Use ContentLength instead.
		if resp.ContentLength > 0 {
			totalBytes = resp.ContentLength
		}
	} else {
		parts := strings.Split(contentRange, "/")

		total := int64(-1)

		// Checking for whether a numbered total exists
		// If one does not exist, we will assume the total to be -1, undefined,
		// and sequentially download each chunk until hitting a 416 error
		totalStr := parts[len(parts)-1]
		if totalStr != "*" {
			total, err = strconv.ParseInt(totalStr, 10, 64)
			if err != nil {
				err = fmt.Errorf("failed extracting file size")
			}
		} else {
			err = fmt.Errorf("file size unknown")
		}

		totalBytes = total
	}
	if totalBytes != d.params.Size && err == nil {
		err = fmt.Errorf("expect file size=%d unmatch remote report size=%d, need refresh cache", d.params.Size, totalBytes)
	}
	if err != nil {
		_ = d.interrupt()
		d.setErr(err)
	}
	return err

}

func (d *downloader) incrWritten(n int64) {
	d.m.Lock()
	defer d.m.Unlock()

	d.written += n
}

// getErr is a thread-safe getter for the error object
func (d *downloader) getErr() error {
	d.m.Lock()
	defer d.m.Unlock()

	return d.err
}

// setErr is a thread-safe setter for the error object
func (d *downloader) setErr(e error) {
	d.m.Lock()
	defer d.m.Unlock()

	d.err = e
}

// Chunk represents a single chunk of data to write by the worker routine.
// This structure also implements an io.SectionReader style interface for
// io.WriterAt, effectively making it an io.SectionWriter (which does not
// exist).
type chunk struct {
	start int64
	size  int64
	buf   *Buf
	id    int

	// Downloader takes range (start,length), but this chunk is requesting equal/sub range of it.
	// To convert the writer to reader eventually, we need to write within the boundary
	//boundary http_range.Range
}

func DefaultHttpRequestFunc(ctx context.Context, params *HttpRequestParams) (*http.Response, error) {
	header := http_range.ApplyRangeToHttpHeader(params.Range, params.HeaderRef)

	res, err := RequestHttp(ctx, "GET", header, params.URL)
	if err != nil {
		return nil, err
	}
	return res, nil
}

type HttpRequestParams struct {
	URL string
	//only want data within this range
	Range     http_range.Range
	HeaderRef http.Header
	//total file size
	Size int64
}
type errReadingBody struct {
	err error
}

func (e *errReadingBody) Error() string {
	return fmt.Sprintf("failed to read part body: %v", e.err)
}

func (e *errReadingBody) Unwrap() error {
	return e.err
}

type MultiReadCloser struct {
	cfg    *cfg
	closer closerFunc
	finish finishBufFUnc
}

type cfg struct {
	rPos   int //current reader position, start from 0
	curBuf *Buf
}

type closerFunc func() error
type finishBufFUnc func(id int) (isLast bool, buf *Buf)

// NewMultiReadCloser to save memory, we re-use limited Buf, and feed data to Read()
func NewMultiReadCloser(buf *Buf, c closerFunc, fb finishBufFUnc) *MultiReadCloser {
	return &MultiReadCloser{closer: c, finish: fb, cfg: &cfg{curBuf: buf}}
}

func (mr MultiReadCloser) Read(p []byte) (n int, err error) {
	if mr.cfg.curBuf == nil {
		return 0, io.EOF
	}
	n, err = mr.cfg.curBuf.Read(p)
	//log.Debugf("read_%d read current buffer, n=%d ,err=%+v", mr.cfg.rPos, n, err)
	if err == io.EOF {
		log.Debugf("read_%d finished current buffer", mr.cfg.rPos)

		isLast, next := mr.finish(mr.cfg.rPos)
		if isLast {
			return n, io.EOF
		}
		mr.cfg.curBuf = next
		mr.cfg.rPos++
		//current.Close()
		return n, nil
	}
	return n, err
}
func (mr MultiReadCloser) Close() error {
	return mr.closer()
}

type Buf struct {
	buffer *bytes.Buffer
	size   int //expected size
	ctx    context.Context
	off    int
	rw     sync.Mutex
	//notify chan struct{}
}

// NewBuf is a buffer that can have 1 read & 1 write at the same time.
// when read is faster write, immediately feed data to read after written
func NewBuf(ctx context.Context, maxSize int, id int) *Buf {
	d := make([]byte, 0, maxSize)
	return &Buf{
		ctx:    ctx,
		buffer: bytes.NewBuffer(d),
		size:   maxSize,
		//notify: make(chan struct{}),
	}
}
func (br *Buf) Reset(size int) {
	br.buffer.Reset()
	br.size = size
	br.off = 0
}

func (br *Buf) Read(p []byte) (n int, err error) {
	if err := br.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if br.off >= br.size {
		return 0, io.EOF
	}
	br.rw.Lock()
	n, err = br.buffer.Read(p)
	br.rw.Unlock()
	if err == nil {
		br.off += n
		return n, err
	}
	if err != io.EOF {
		return n, err
	}
	if n != 0 {
		br.off += n
		return n, nil
	}
	// n==0, err==io.EOF
	// wait for new write for 200ms
	select {
	case <-br.ctx.Done():
		return 0, br.ctx.Err()
	//case <-br.notify:
	//	return 0, nil
	case <-time.After(time.Millisecond * 200):
		return 0, nil
	}
}

func (br *Buf) Write(p []byte) (n int, err error) {
	if err := br.ctx.Err(); err != nil {
		return 0, err
	}
	br.rw.Lock()
	defer br.rw.Unlock()
	n, err = br.buffer.Write(p)
	select {
	//case br.notify <- struct{}{}:
	default:
	}
	return
}

func (br *Buf) Close() {
	//close(br.notify)
}
