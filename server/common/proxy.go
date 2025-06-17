package common

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"maps"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/net"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
)

func Proxy(w http.ResponseWriter, r *http.Request, link *model.Link, file model.Obj) error {
	if link.MFile != nil {
		defer link.MFile.Close()
		attachHeader(w, file)
		contentType := link.Header.Get("Content-Type")
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		mFile := link.MFile
		if _, ok := mFile.(*os.File); !ok {
			mFile = &stream.RateLimitFile{
				File:    mFile,
				Limiter: stream.ServerDownloadLimit,
				Ctx:     r.Context(),
			}
		}
		http.ServeContent(w, r, file.GetName(), file.ModTime(), mFile)
		return nil
	} else if link.RangeReadCloser != nil {
		attachHeader(w, file)
		return net.ServeHTTP(w, r, file.GetName(), file.ModTime(), file.GetSize(), &stream.RateLimitRangeReadCloser{
			RangeReadCloserIF: link.RangeReadCloser,
			Limiter:           stream.ServerDownloadLimit,
		})
	} else if link.Concurrency != 0 || link.PartSize != 0 {
		attachHeader(w, file)
		size := file.GetSize()
		rangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
			requestHeader := ctx.Value("request_header")
			if requestHeader == nil {
				requestHeader = http.Header{}
			}
			header := net.ProcessHeader(requestHeader.(http.Header), link.Header)
			down := net.NewDownloader(func(d *net.Downloader) {
				d.Concurrency = link.Concurrency
				d.PartSize = link.PartSize
			})
			req := &net.HttpRequestParams{
				URL:       link.URL,
				Range:     httpRange,
				Size:      size,
				HeaderRef: header,
			}
			rc, err := down.Download(ctx, req)
			return rc, err
		}
		return net.ServeHTTP(w, r, file.GetName(), file.ModTime(), file.GetSize(), &stream.RateLimitRangeReadCloser{
			RangeReadCloserIF: &model.RangeReadCloser{RangeReader: rangeReader},
			Limiter:           stream.ServerDownloadLimit,
		})
	} else {
		//transparent proxy
		header := net.ProcessHeader(r.Header, link.Header)
		res, err := net.RequestHttp(r.Context(), r.Method, header, link.URL)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		maps.Copy(w.Header(), res.Header)
		w.WriteHeader(res.StatusCode)
		if r.Method == http.MethodHead {
			return nil
		}
		_, err = utils.CopyWithBuffer(w, &stream.RateLimitReader{
			Reader:  res.Body,
			Limiter: stream.ServerDownloadLimit,
			Ctx:     r.Context(),
		})
		return err
	}
}
func attachHeader(w http.ResponseWriter, file model.Obj) {
	fileName := file.GetName()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fileName, url.PathEscape(fileName)))
	w.Header().Set("Content-Type", utils.GetMimeType(fileName))
	w.Header().Set("Etag", GetEtag(file))
}
func GetEtag(file model.Obj) string {
	hash := ""
	for _, v := range file.GetHash().Export() {
		if strings.Compare(v, hash) > 0 {
			hash = v
		}
	}
	if len(hash) > 0 {
		return fmt.Sprintf(`"%s"`, hash)
	}
	// 参考nginx
	return fmt.Sprintf(`"%x-%x"`, file.ModTime().Unix(), file.GetSize())
}

var NoProxyRange = &model.RangeReadCloser{}

func ProxyRange(link *model.Link, size int64) {
	if link.MFile != nil {
		return
	}
	if link.RangeReadCloser == nil {
		var rrc, err = stream.GetRangeReadCloserFromLink(size, link)
		if err != nil {
			log.Warnf("ProxyRange error: %s", err)
			return
		}
		link.RangeReadCloser = rrc
	} else if link.RangeReadCloser == NoProxyRange {
		link.RangeReadCloser = nil
	}
}

type InterceptResponseWriter struct {
	http.ResponseWriter
	io.Writer
}

func (iw *InterceptResponseWriter) Write(p []byte) (int, error) {
	return iw.Writer.Write(p)
}

type WrittenResponseWriter struct {
	http.ResponseWriter
	written bool
}

func (ww *WrittenResponseWriter) Write(p []byte) (int, error) {
	n, err := ww.ResponseWriter.Write(p)
	if !ww.written && n > 0 {
		ww.written = true
	}
	return n, err
}

func (ww *WrittenResponseWriter) IsWritten() bool {
	return ww.written
}
