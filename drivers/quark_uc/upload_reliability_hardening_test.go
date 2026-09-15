package quark

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func ossResponse(req *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func uploadTestPre() UpPreResp {
	var pre UpPreResp
	pre.Data.TaskId = "task-test"
	pre.Data.UploadId = "upload-test"
	pre.Data.ObjKey = "object-test"
	pre.Data.UploadUrl = "https://oss.test"
	pre.Data.Bucket = "bucket"
	pre.Data.AuthInfo = "auth-info"
	return pre
}

func TestRetryClassifierRejectsTokenExtensions(t *testing.T) {
	for _, msg := range []string{
		"complete_upload_lock_timeout_but_other",
		"inner errors, requestId abc",
	} {
		if isRetryableQuarkUploadControlError(errors.New(msg)) {
			t.Fatalf("token extension must not be retryable: %q", msg)
		}
	}
}

func TestFindUploadedFileByFIDStopsAtPageCap(t *testing.T) {
	calls := 0
	fullPage := make([]map[string]any, 0, 100)
	for i := 0; i < 100; i++ {
		fullPage = append(fullPage, map[string]any{
			"fid":       "other",
			"file_name": "other",
			"file":      true,
			"size":      1,
		})
	}

	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/1/clouddrive/file/sort" {
			http.NotFound(w, r)
			return
		}
		calls++
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"list": fullPage},
			"metadata": map[string]any{
				"_size": 100, "_page": calls, "_count": 100, "_total": 0,
			},
		})
	}))

	d := newTestDriver(srv.URL)
	visible, err := d.findUploadedFileByFID(context.Background(), "parent", "target")
	if visible {
		t.Fatal("unexpected visible target")
	}
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("err=%v, want page-cap error", err)
	}
	if calls != uploadFinishVisibilityMaxPages {
		t.Fatalf("calls=%d, want %d", calls, uploadFinishVisibilityMaxPages)
	}
}

func TestFindUploadedFileByFIDCancelsInFlightRequest(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/sort" {
			http.NotFound(w, r)
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
	}))

	d := newTestDriver(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := d.findUploadedFileByFID(ctx, "parent", "target")
		done <- err
	}()

	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("visibility request did not reach server")
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight visibility request did not cancel")
	}
}

func TestUpPartReliableRetriesAuthBeforeSendingOSSOnce(t *testing.T) {
	authCalls := 0
	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/1/clouddrive/file/upload/auth" {
			http.NotFound(w, r)
			return
		}
		authCalls++
		if authCalls == 1 {
			writeJSON(w, http.StatusInternalServerError, Resp{Status: 500, Code: 500, Message: "inner error, requestId auth-1"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"auth_key": "key"},
		})
	}))

	oldClient := base.RestyClient
	defer func() { base.RestyClient = oldClient }()
	ossCalls := 0
	var gotBody string
	base.RestyClient = resty.NewWithClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ossCalls++
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		gotBody = string(body)
		headers := make(http.Header)
		headers.Set("Etag", "etag-1")
		return ossResponse(req, http.StatusOK, "", headers), nil
	})})

	d := newTestDriver(srv.URL)
	part := []byte("full-part-body")
	etag, err := d.upPartReliable(context.Background(), uploadTestPre(), "application/octet-stream", 1, part)
	if err != nil {
		t.Fatalf("upPartReliable: %v", err)
	}
	if authCalls != 2 || ossCalls != 1 {
		t.Fatalf("authCalls=%d ossCalls=%d, want 2/1", authCalls, ossCalls)
	}
	if gotBody != string(part) {
		t.Fatalf("OSS body=%q, want %q", gotBody, string(part))
	}
	if etag != "etag-1" {
		t.Fatalf("etag=%q, want etag-1", etag)
	}
}

func TestUpPartReliableDoesNotRetryOSSError(t *testing.T) {
	authCalls := 0
	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/upload/auth" {
			http.NotFound(w, r)
			return
		}
		authCalls++
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"auth_key": "key"},
		})
	}))

	oldClient := base.RestyClient
	defer func() { base.RestyClient = oldClient }()
	ossCalls := 0
	base.RestyClient = resty.NewWithClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ossCalls++
		return ossResponse(req, http.StatusInternalServerError, "inner error, requestId oss-1", nil), nil
	})})

	d := newTestDriver(srv.URL)
	_, err := d.upPartReliable(context.Background(), uploadTestPre(), "application/octet-stream", 1, []byte("part"))
	if err == nil || !strings.Contains(err.Error(), "up status: 500") {
		t.Fatalf("err=%v, want OSS status error", err)
	}
	if authCalls != 1 || ossCalls != 1 {
		t.Fatalf("authCalls=%d ossCalls=%d, want 1/1", authCalls, ossCalls)
	}
}

func TestUpCommitReliableRetriesAuthBeforeSendingOSSOnce(t *testing.T) {
	authCalls := 0
	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/upload/auth" {
			http.NotFound(w, r)
			return
		}
		authCalls++
		if authCalls == 1 {
			writeJSON(w, http.StatusInternalServerError, Resp{Status: 500, Code: 500, Message: "complete_upload_lock_timeout"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"auth_key": "key"},
		})
	}))

	oldClient := base.RestyClient
	defer func() { base.RestyClient = oldClient }()
	ossCalls := 0
	base.RestyClient = resty.NewWithClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ossCalls++
		return ossResponse(req, http.StatusOK, "", nil), nil
	})})

	d := newTestDriver(srv.URL)
	if err := d.upCommitReliable(context.Background(), uploadTestPre(), []string{"etag-1"}); err != nil {
		t.Fatalf("upCommitReliable: %v", err)
	}
	if authCalls != 2 || ossCalls != 1 {
		t.Fatalf("authCalls=%d ossCalls=%d, want 2/1", authCalls, ossCalls)
	}
}

func TestUpCommitReliableDoesNotRetryOSSError(t *testing.T) {
	authCalls := 0
	srv := httptestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/upload/auth" {
			http.NotFound(w, r)
			return
		}
		authCalls++
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"auth_key": "key"},
		})
	}))

	oldClient := base.RestyClient
	defer func() { base.RestyClient = oldClient }()
	ossCalls := 0
	base.RestyClient = resty.NewWithClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ossCalls++
		return ossResponse(req, http.StatusInternalServerError, "inner error, requestId oss-commit", nil), nil
	})})

	d := newTestDriver(srv.URL)
	err := d.upCommitReliable(context.Background(), uploadTestPre(), []string{"etag-1"})
	if err == nil || !strings.Contains(err.Error(), "up status: 500") {
		t.Fatalf("err=%v, want OSS status error", err)
	}
	if authCalls != 1 || ossCalls != 1 {
		t.Fatalf("authCalls=%d ossCalls=%d, want 1/1", authCalls, ossCalls)
	}
}

func httptestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}
