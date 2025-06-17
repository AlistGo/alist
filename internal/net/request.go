package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
const DefaultDownloadPartSize = utils.MB * 10

// DefaultDownloadConcurrency is the default number of goroutines to spin up
// when using Download().
const DefaultDownloadConcurrency = 2

// DefaultPartBodyMaxRetries is the default number of retries to make when a part fails to download.
const DefaultPartBodyMaxRetries = 3

var DefaultConcurrencyLimit *ConcurrencyLimit

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

	*ConcurrencyLimit
}
type HttpRequestFunc func(ctx context.Context, params *HttpRequestParams) (*http.Response, error)

func NewDownloader(options ...func(*Downloader)) *Downloader {
	d := &Downloader{ //允许不设置的选项
		PartBodyMaxRetries: DefaultPartBodyMaxRetries,
		ConcurrencyLimit:   DefaultConcurrencyLimit,
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
	// 必需的选项
	if impl.cfg.Concurrency == 0 {
		impl.cfg.Concurrency = DefaultDownloadConcurrency
	}
	if impl.cfg.PartSize == 0 {
		impl.cfg.PartSize = DefaultDownloadPartSize
	}
	if impl.cfg.HttpClient == nil {
		impl.cfg.HttpClient = DefaultHttpRequestFunc
	}

	return impl.download()
}

// downloader is the implementation structure used internally by Downloader.
type downloader struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	cfg    Downloader

	params       *HttpRequestParams //http request params
	chunkChannel chan chunk         //chunk chanel

	//wg sync.WaitGroup
	m sync.Mutex

	nextChunk int //next chunk id
	bufs      []*Buf
	written   int64 //total bytes of file downloaded from remote
	err       error

	concurrency int //剩余的并发数，递减。到0时停止并发
	maxPart     int //有多少个分片
	pos         int64
	maxPos      int64
	m2          sync.Mutex
	readingID   int // 正在被读取的id
}

type ConcurrencyLimit struct {
	_m    sync.Mutex
	Limit int // 需要大于0
}

var ErrExceedMaxConcurrency = fmt.Errorf("ExceedMaxConcurrency")

func (l *ConcurrencyLimit) sub() error {
	l._m.Lock()
	defer l._m.Unlock()
	if l.Limit-1 < 0 {
		return ErrExceedMaxConcurrency
	}
	l.Limit--
	// log.Debugf("ConcurrencyLimit.sub: %d", l.Limit)
	return nil
}
func (l *ConcurrencyLimit) add() {
	l._m.Lock()
	defer l._m.Unlock()
	l.Limit++
	// log.Debugf("ConcurrencyLimit.add: %d", l.Limit)
}

// 检测是否超过限制
func (d *downloader) concurrencyCheck() error {
	if d.cfg.ConcurrencyLimit != nil {
		return d.cfg.ConcurrencyLimit.sub()
	}
	return nil
}
func (d *downloader) concurrencyFinish() {
	if d.cfg.ConcurrencyLimit != nil {
		d.cfg.ConcurrencyLimit.add()
	}
}

// download performs the implementation of the object download across ranged GETs.
func (d *downloader) download() (io.ReadCloser, error) {
	if err := d.concurrencyCheck(); err != nil {
		return nil, err
	}
	d.ctx, d.cancel = context.WithCancelCause(d.ctx)

	maxPart := int(d.params.Range.Length / int64(d.cfg.PartSize))
	if d.params.Range.Length%int64(d.cfg.PartSize) > 0 {
		maxPart++
	}
	if maxPart < d.cfg.Concurrency {
		d.cfg.Concurrency = maxPart
	}
	log.Debugf("cfgConcurrency:%d", d.cfg.Concurrency)

	if d.cfg.Concurrency == 1 {
		if d.cfg.ConcurrencyLimit != nil {
			go func() {
				<-d.ctx.Done()
				d.concurrencyFinish()
			}()
		}
		resp, err := d.cfg.HttpClient(d.ctx, d.params)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}

	// workers
	d.chunkChannel = make(chan chunk, d.cfg.Concurrency)

	d.maxPart = maxPart
	d.pos = d.params.Range.Start
	d.maxPos = d.params.Range.Start + d.params.Range.Length
	d.concurrency = d.cfg.Concurrency
	d.sendChunkTask(true)

	var rc io.ReadCloser = NewMultiReadCloser(d.bufs[0], d.interrupt, d.finishBuf)

	// Return error
	return rc, d.err
}

func (d *downloader) sendChunkTask(newConcurrency bool) error {
	d.m.Lock()
	defer d.m.Unlock()
	isNewBuf := d.concurrency > 0
	if newConcurrency {
		if d.concurrency <= 0 {
			return nil
		}
		if d.nextChunk > 0 { // 第一个不检查，因为已经检查过了
			if err := d.concurrencyCheck(); err != nil {
				return err
			}
		}
		d.concurrency--
		go d.downloadPart()
	}

	var buf *Buf
	if isNewBuf {
		buf = NewBuf(d.ctx, d.cfg.PartSize)
		d.bufs = append(d.bufs, buf)
	} else {
		buf = d.getBuf(d.nextChunk)
	}

	if d.pos < d.maxPos {
		finalSize := int64(d.cfg.PartSize)
		switch d.nextChunk {
		case 0:
			// 最小分片在前面有助视频播放？
			firstSize := d.params.Range.Length % finalSize
			if firstSize > 0 {
				minSize := finalSize / 2
				if firstSize < minSize { // 最小分片太小就调整到一半
					finalSize = minSize
				} else {
					finalSize = firstSize
				}
			}
		case 1:
			firstSize := d.params.Range.Length % finalSize
			minSize := finalSize / 2
			if firstSize > 0 && firstSize < minSize {
				finalSize += firstSize - minSize
			}
		}
		buf.Reset(int(finalSize))
		ch := chunk{
			start: d.pos,
			size:  finalSize,
			id:    d.nextChunk,
			buf:   buf,

			newConcurrency: newConcurrency,
		}
		d.pos += finalSize
		d.nextChunk++
		d.chunkChannel <- ch
		return nil
	}
	return nil
}

// when the final reader Close, we interrupt
func (d *downloader) interrupt() error {
	if d.written != d.params.Range.Length {
		log.Debugf("Downloader interrupt before finish")
		if d.getErr() == nil {
			d.setErr(fmt.Errorf("interrupted"))
		}
	}
	d.cancel(d.err)
	defer func() {
		close(d.chunkChannel)
		for _, buf := range d.bufs {
			buf.Close()
		}
		if d.concurrency > 0 {
			d.concurrency = -d.concurrency
		}
		log.Debugf("maxConcurrency:%d", d.cfg.Concurrency+d.concurrency)
	}()
	return d.err
}
func (d *downloader) getBuf(id int) (b *Buf) {
	return d.bufs[id%len(d.bufs)]
}
func (d *downloader) finishBuf(id int) (isLast bool, nextBuf *Buf) {
	id++
	if id >= d.maxPart {
		return true, nil
	}

	d.sendChunkTask(false)

	d.readingID = id
	return false, d.getBuf(id)
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
			break
		}
		if err := d.downloadChunk(&c); err != nil {
			if err == errCancelConcurrency {
				break
			}
			if err == context.Canceled {
				if e := context.Cause(d.ctx); e != nil {
					err = e
				}
			}
			d.setErr(err)
			d.cancel(err)
		}
	}
	d.concurrencyFinish()
}

// downloadChunk downloads the chunk
func (d *downloader) downloadChunk(ch *chunk) error {
	log.Debugf("start chunk_%d, %+v", ch.id, ch)
	params := d.getParamsFromChunk(ch)
	var n int64
	var err error
	for retry := 0; retry <= d.cfg.PartBodyMaxRetries; retry++ {
		if d.getErr() != nil {
			return nil
		}
		n, err = d.tryDownloadChunk(params, ch)
		if err == nil {
			d.incrWritten(n)
			log.Debugf("chunk_%d downloaded", ch.id)
			break
		}
		if d.getErr() != nil {
			return nil
		}
		if utils.IsCanceled(d.ctx) {
			return d.ctx.Err()
		}
		// Check if the returned error is an errNeedRetry.
		// If this occurs we unwrap the err to set the underlying error
		// and attempt any remaining retries.
		if e, ok := err.(*errNeedRetry); ok {
			err = e.Unwrap()
			if n > 0 {
				// 测试：下载时 断开 alist向云盘发起的下载连接
				// 校验：下载完后校验文件哈希值 一致
				d.incrWritten(n)
				ch.start += n
				ch.size -= n
				params.Range.Start = ch.start
				params.Range.Length = ch.size
			}
			log.Warnf("err chunk_%d, object part download error %s, retrying attempt %d. %v",
				ch.id, params.URL, retry, err)
		} else if err == errInfiniteRetry {
			retry--
			continue
		} else {
			break
		}
	}

	return err
}

var errCancelConcurrency = fmt.Errorf("cancel concurrency")
var errInfiniteRetry = fmt.Errorf("infinite retry")

func (d *downloader) tryDownloadChunk(params *HttpRequestParams, ch *chunk) (int64, error) {
	resp, err := d.cfg.HttpClient(d.ctx, params)
	if err != nil {
		if resp == nil {
			return 0, err
		}
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return 0, err
		}
		if ch.id == 0 { //第1个任务 有限的重试，超过重试就会结束请求
			switch resp.StatusCode {
			default:
				return 0, err
			case http.StatusTooManyRequests:
			case http.StatusBadGateway:
			case http.StatusServiceUnavailable:
			case http.StatusGatewayTimeout:
			}
			<-time.After(time.Millisecond * 200)
			return 0, &errNeedRetry{err: fmt.Errorf("http request failure,status: %d", resp.StatusCode)}
		}

		// 来到这 说明第1个分片下载 连接成功了
		// 后续分片下载出错都当超载处理
		log.Debugf("err chunk_%d, try downloading:%v", ch.id, err)

		d.m.Lock()
		isCancelConcurrency := ch.newConcurrency
		if d.concurrency > 0 { // 取消剩余的并发任务
			// 用于计算实际的并发数
			d.concurrency = -d.concurrency
			isCancelConcurrency = true
		}
		if isCancelConcurrency {
			d.concurrency--
			d.chunkChannel <- *ch
			d.m.Unlock()
			return 0, errCancelConcurrency
		}
		d.m.Unlock()
		if ch.id != d.readingID { //正在被读取的优先重试
			d.m2.Lock()
			defer d.m2.Unlock()
			<-time.After(time.Millisecond * 200)
		}
		return 0, errInfiniteRetry
	}
	defer resp.Body.Close()
	//only check file size on the first task
	if ch.id == 0 {
		err = d.checkTotalBytes(resp)
		if err != nil {
			return 0, err
		}
	}
	d.sendChunkTask(true)
	n, err := utils.CopyWithBuffer(ch.buf, resp.Body)

	if err != nil {
		return n, &errNeedRetry{err: err}
	}
	if n != ch.size {
		err = fmt.Errorf("chunk download size incorrect, expected=%d, got=%d", ch.size, n)
		return n, &errNeedRetry{err: err}
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
	totalBytes := int64(-1)
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
		// _ = d.interrupt()
		d.setErr(err)
		d.cancel(err)
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

	newConcurrency bool
}

func DefaultHttpRequestFunc(ctx context.Context, params *HttpRequestParams) (*http.Response, error) {
	header := http_range.ApplyRangeToHttpHeader(params.Range, params.HeaderRef)

	res, err := RequestHttp(ctx, "GET", header, params.URL)
	if err != nil {
		return res, err
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
type errNeedRetry struct {
	err error
}

func (e *errNeedRetry) Error() string {
	return e.err.Error()
}

func (e *errNeedRetry) Unwrap() error {
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
		return n, nil
	}
	if err == context.Canceled {
		if e := context.Cause(mr.cfg.curBuf.ctx); e != nil {
			err = e
		}
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
}

// NewBuf is a buffer that can have 1 read & 1 write at the same time.
// when read is faster write, immediately feed data to read after written
func NewBuf(ctx context.Context, maxSize int) *Buf {
	return &Buf{
		ctx:    ctx,
		buffer: bytes.NewBuffer(make([]byte, 0, maxSize)),
		size:   maxSize,
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
	return
}

func (br *Buf) Close() {
	br.buffer = nil
}
