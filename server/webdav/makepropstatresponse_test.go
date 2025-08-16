package webdav

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/pkg/utils"
)

func TestMakePropstatResponseKeepsEncodedHref(t *testing.T) {
	// Non-ASCII directory path
	dir := "/测试"
	href := utils.EncodePath(dir, true)
	ps := []Propstat{{Status: http.StatusOK}}

	rec := httptest.NewRecorder()
	mw := multistatusWriter{w: rec}
	if err := mw.write(makePropstatResponse(href, ps)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if rec.Code != StatusMulti {
		t.Fatalf("status = %d, want %d", rec.Code, StatusMulti)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<D:href>"+href+"</D:href>") {
		t.Fatalf("href not preserved: got body %q", body)
	}
}
