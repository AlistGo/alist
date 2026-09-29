package quark

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestRemoveReliableRejectsUnprovenDeleteResponses(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "http 500 empty json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeDeleteJSON(w, http.StatusInternalServerError, map[string]any{})
			},
		},
		{
			name: "http 500 incomplete envelope",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeDeleteJSON(w, http.StatusInternalServerError, map[string]any{"status": 500})
			},
		},
		{
			name: "http 500 html",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("<html><body>500</body></html>"))
			},
		},
		{
			name: "http 200 provider rejection",
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeDeleteJSON(w, http.StatusOK, map[string]any{
					"status":  400,
					"code":    14001,
					"message": "bad parameter",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != fileDeletePath {
					http.NotFound(w, r)
					return
				}
				deleteCalls++
				tt.handler(w, r)
			}))
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			if err := d.Remove(context.Background(), deleteTestObject("fid-delete-response")); err == nil {
				t.Fatal("Remove must not report success from an unproven delete response")
			}
			if deleteCalls != 1 {
				t.Fatalf("deleteCalls=%d, want 1", deleteCalls)
			}
		})
	}
}

func TestDeleteFileExistsByFIDRequiresHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusCreated, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"fid": "target-fid"},
		})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err == nil {
		t.Fatal("HTTP 201 must not satisfy the PRESENT contract")
	}
	if exists {
		t.Fatal("HTTP/provider status disagreement must not report present")
	}
}

type failFirstDeleteResponseTransport struct {
	base http.RoundTripper

	mu     sync.Mutex
	failed bool
}

func (t *failFirstDeleteResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if req.Method != http.MethodPost || req.URL.Path != fileDeletePath {
		return resp, nil
	}

	t.mu.Lock()
	if t.failed {
		t.mu.Unlock()
		return resp, nil
	}
	t.failed = true
	t.mu.Unlock()

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return nil, errors.New("simulated lost delete response")
}

func TestRemoveReliableVerifiesBeforeTransportReplayWithProductionRetryCount(t *testing.T) {
	var (
		mu       sync.Mutex
		sequence []string
		deletes  int
		infos    int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			deletes++
			sequence = append(sequence, "delete")
			writeDeleteJSON(w, http.StatusOK, map[string]any{"status": 200, "code": 0})
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			infos++
			sequence = append(sequence, "info")
			fileInfoPresent("fid-transport")(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := &failFirstDeleteResponseTransport{base: http.DefaultTransport}
	productionLikeClient := resty.NewWithClient(&http.Client{Transport: transport}).SetRetryCount(3)
	d := newDeleteTestDriver(srv.URL)
	d.client = productionLikeClient

	if err := d.Remove(context.Background(), deleteTestObject("fid-transport")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	mu.Lock()
	gotSequence := append([]string(nil), sequence...)
	gotDeletes, gotInfos := deletes, infos
	mu.Unlock()
	wantSequence := []string{"delete", "info", "delete"}
	if !reflect.DeepEqual(gotSequence, wantSequence) {
		t.Fatalf("request sequence=%v, want %v", gotSequence, wantSequence)
	}
	if gotDeletes != 2 || gotInfos != 1 {
		t.Fatalf("deletes=%d infos=%d, want 2/1", gotDeletes, gotInfos)
	}
}

func TestRemoveReliableTransportAmbiguityDoesNotReplayWhenVerifierFails(t *testing.T) {
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			deleteCalls++
			writeDeleteJSON(w, http.StatusOK, map[string]any{"status": 200, "code": 0})
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			infoCalls++
			writeDeleteJSON(w, http.StatusTooManyRequests, map[string]any{
				"status": 429, "code": 32003, "message": "too many requests",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := &failFirstDeleteResponseTransport{base: http.DefaultTransport}
	productionLikeClient := resty.NewWithClient(&http.Client{Transport: transport}).SetRetryCount(3)
	d := newDeleteTestDriver(srv.URL)
	d.client = productionLikeClient

	if err := d.Remove(context.Background(), deleteTestObject("fid-transport-verify-error")); err == nil {
		t.Fatal("Remove must fail closed when transport outcome is ambiguous and verification fails")
	}
	if deleteCalls != 1 || infoCalls != 1 {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want 1/1", deleteCalls, infoCalls)
	}
}
