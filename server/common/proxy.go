package common

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/net"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/microcosm-cc/bluemonday"
	log "github.com/sirupsen/logrus"
	"github.com/yuin/goldmark"
)

func processMarkdown(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := goldmark.New().Convert(content, &buf); err != nil {
		return nil, fmt.Errorf("markdown conversion failed: %w", err)
	}
	return bluemonday.UGCPolicy().SanitizeBytes(buf.Bytes()), nil
}

func Proxy(w http.ResponseWriter, r *http.Request, link *model.Link, file model.Obj) error {

	//优先处理md文件
	if utils.Ext(file.GetName()) == "md" && setting.GetBool(conf.FilterReadMeScripts) {
		var markdownContent []byte
		var err error

		if link.MFile != nil {
			defer link.MFile.Close()
			attachFileName(w, file)
			markdownContent, err = io.ReadAll(link.MFile)
			if err != nil {
				return fmt.Errorf("failed to read markdown content: %w", err)
			}

		} else {
			header := net.ProcessHeader(r.Header, link.Header)
			res, err := net.RequestHttp(r.Context(), r.Method, header, link.URL)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			for h, v := range res.Header {
				w.Header()[h] = v
			}
			w.WriteHeader(res.StatusCode)
			if r.Method == http.MethodHead {
				return nil
			}
			markdownContent, err = io.ReadAll(res.Body)
			if err != nil {
				return fmt.Errorf("failed to read markdown content: %w", err)
			}

		}

		safeHTML, err := processMarkdown(markdownContent)
		if err != nil {
			return err
		}

		safeHTMLReader := bytes.NewReader(safeHTML)
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(safeHTML)), 10))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err = utils.CopyWithBuffer(w, safeHTMLReader)
		if err != nil {
			return err
		}
		return nil
	}

	if link.MFile != nil {
		defer link.MFile.Close()
		attachFileName(w, file)
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
		attachFileName(w, file)
		net.ServeHTTP(w, r, file.GetName(), file.ModTime(), file.GetSize(), &stream.RateLimitRangeReadCloser{
			RangeReadCloserIF: link.RangeReadCloser,
			Limiter:           stream.ServerDownloadLimit,
		})
		return nil
	} else if link.Concurrency != 0 || link.PartSize != 0 {
		attachFileName(w, file)
		size := file.GetSize()
		header := net.ProcessHeader(r.Header, link.Header)
		rangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
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
		net.ServeHTTP(w, r, file.GetName(), file.ModTime(), file.GetSize(), &stream.RateLimitRangeReadCloser{
			RangeReadCloserIF: &model.RangeReadCloser{RangeReader: rangeReader},
			Limiter:           stream.ServerDownloadLimit,
		})
		return nil
	} else {
		//transparent proxy
		header := net.ProcessHeader(r.Header, link.Header)
		res, err := net.RequestHttp(r.Context(), r.Method, header, link.URL)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		for h, v := range res.Header {
			w.Header()[h] = v
		}
		w.WriteHeader(res.StatusCode)
		if r.Method == http.MethodHead {
			return nil
		}
		_, err = utils.CopyWithBuffer(w, &stream.RateLimitReader{
			Reader:  res.Body,
			Limiter: stream.ServerDownloadLimit,
			Ctx:     r.Context(),
		})
		if err != nil {
			return err
		}
		return nil
	}
}

func attachFileName(w http.ResponseWriter, file model.Obj) {
	fileName := file.GetName()
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fileName, url.PathEscape(fileName)))
	w.Header().Set("Content-Type", utils.GetMimeType(fileName))
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
