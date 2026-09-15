package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
)

func TestIsInlineSafeContentType(t *testing.T) {
	datas := []struct {
		contentType string
		result      bool
	}{
		{"text/html", false},
		{"text/html; charset=utf-8", false},
		{"application/xhtml+xml", false},
		{"image/svg+xml", false},
		{"IMAGE/SVG+XML", false},
		{"image/png", true},
		{"application/pdf", true},
		{"text/plain", true},
		{"", true},
	}
	for i, data := range datas {
		if got := isInlineSafeContentType(data.contentType); got != data.result {
			t.Errorf("isInlineSafeContentType %d (%q) = %v, want %v", i, data.contentType, got, data.result)
		}
	}
}

// TestProxyTransparentPreviewBlocksHtml reproduces GHSA-fq52-hpvg-r5qv: a
// transparently proxied file previewed via ?type=preview must never be
// served as inline text/html, regardless of what Content-Type the upstream
// storage returns.
func TestProxyTransparentPreviewBlocksHtml(t *testing.T) {
	if conf.Conf == nil {
		conf.Conf = &conf.Config{}
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<script>alert(1)</script>"))
	}))
	defer upstream.Close()

	link := &model.Link{URL: upstream.URL}
	file := &model.Object{Name: "evil.html"}

	req := httptest.NewRequest(http.MethodGet, "/d/evil.html?type=preview", nil)
	rec := httptest.NewRecorder()

	if err := Proxy(rec, req, link, file); err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment disposition (preview of html must not be inline)", disposition)
	}
}

// TestProxyTransparentPreviewKeepsUpstreamContentType reproduces the
// doubao_new page-thumbnail preview: the upstream CDN serves an image/webp
// page thumbnail for a file whose own name ends in .pdf. The proxy must keep
// serving the thumbnail inline using the upstream Content-Type instead of
// deriving one from the file name, or the preview breaks.
func TestProxyTransparentPreviewKeepsUpstreamContentType(t *testing.T) {
	if conf.Conf == nil {
		conf.Conf = &conf.Config{}
	}

	webpBody := []byte("RIFF....WEBPVP8 fakepayload")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		_, _ = w.Write(webpBody)
	}))
	defer upstream.Close()

	link := &model.Link{URL: upstream.URL}
	file := &model.Object{Name: "report.pdf"}

	req := httptest.NewRequest(http.MethodGet, "/d/report.pdf?type=preview", nil)
	rec := httptest.NewRecorder()

	if err := Proxy(rec, req, link, file); err != nil {
		t.Fatalf("Proxy returned error: %v", err)
	}

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Errorf("Content-Type = %q, want %q (must not be derived from the file name)", got, "image/webp")
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "inline") {
		t.Errorf("Content-Disposition = %q, want an inline disposition (webp thumbnail preview must render)", disposition)
	}
	if rec.Body.String() != string(webpBody) {
		t.Errorf("response body = %q, want the proxied upstream body %q", rec.Body.String(), string(webpBody))
	}
}
