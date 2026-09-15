package quark

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
)

func newDeleteTestDriver(serverURL string) *QuarkOrUC {
	client := resty.New()
	client.SetRetryCount(0)
	return &QuarkOrUC{
		Addition: Addition{Cookie: "test-cookie"},
		conf: Conf{
			api:     serverURL + "/1/clouddrive",
			pr:      "ucpro",
			referer: "https://pan.quark.cn",
		},
		client: client,
	}
}

func writeDeleteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func deleteTestObject(fid string) model.Obj {
	return &model.Object{ID: fid, Name: "chunk.bucket.4"}
}

func TestIsRetryableQuarkDeleteError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "observed provider error", err: errors.New("inner error, requestId 95sg27-abc"), want: true},
		{name: "case insensitive", err: errors.New("Inner Error: RequestId abc"), want: true},
		{name: "missing request id", err: errors.New("inner error"), want: false},
		{name: "token extension plural", err: errors.New("inner errors, requestId abc"), want: false},
		{name: "token extension suffix", err: errors.New("inner error_x, requestId abc"), want: false},
		{name: "permission", err: errors.New("permission denied"), want: false},
		{name: "transport timeout", err: errors.New("context deadline exceeded"), want: false},
		{name: "wrapped unrelated", err: errors.New("delete status: 500, inner error, requestId abc"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableQuarkDeleteError(tt.err); got != tt.want {
				t.Fatalf("isRetryableQuarkDeleteError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRemoveReliableRetriesTransientWhenFIDStillExists(t *testing.T) {
	deleteCalls := 0
	infoCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/delete":
			deleteCalls++
			if deleteCalls == 1 {
				writeDeleteJSON(w, http.StatusInternalServerError, Resp{
					Status: 500, Code: 500, Message: "inner error, requestId delete-1",
				})
				return
			}
			writeDeleteJSON(w, http.StatusOK, Resp{Status: 200, Code: 0})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file":
			infoCalls++
			if got := r.URL.Query().Get("fids"); got != "fid-1" {
				t.Fatalf("fids=%q, want fid-1", got)
			}
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{"list": []map[string]any{{
					"fid": "fid-1", "file_name": "chunk.bucket.4", "file": true,
				}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	if err := d.Remove(context.Background(), deleteTestObject("fid-1")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if deleteCalls != 2 || infoCalls != 1 {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want 2/1", deleteCalls, infoCalls)
	}
}

func TestRemoveReliableTreatsConfirmedAbsentFIDAsSuccess(t *testing.T) {
	deleteCalls := 0
	infoCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/delete":
			deleteCalls++
			writeDeleteJSON(w, http.StatusInternalServerError, Resp{
				Status: 500, Code: 500, Message: "inner error, requestId ambiguous-1",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file":
			infoCalls++
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": []any{}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	if err := d.Remove(context.Background(), deleteTestObject("fid-gone")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if deleteCalls != 1 || infoCalls != 1 {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want 1/1", deleteCalls, infoCalls)
	}
}

func TestRemoveReliableRecoversIfRetrySeesNonTransientAfterEarlierTransient(t *testing.T) {
	deleteCalls := 0
	infoCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/delete":
			deleteCalls++
			if deleteCalls == 1 {
				writeDeleteJSON(w, http.StatusInternalServerError, Resp{
					Status: 500, Code: 500, Message: "inner error, requestId ambiguous-2",
				})
				return
			}
			writeDeleteJSON(w, http.StatusBadRequest, Resp{
				Status: 400, Code: 400, Message: "object not found",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file":
			infoCalls++
			if infoCalls == 1 {
				writeDeleteJSON(w, http.StatusOK, map[string]any{
					"status": 200,
					"code":   0,
					"data": map[string]any{"list": []map[string]any{{
						"fid": "fid-2", "file_name": "chunk.index.4", "file": true,
					}}},
				})
				return
			}
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": []any{}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	if err := d.Remove(context.Background(), deleteTestObject("fid-2")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if deleteCalls != 2 || infoCalls != 2 {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want 2/2", deleteCalls, infoCalls)
	}
}

func TestRemoveReliablePreservesNonTransientErrorWithoutProbe(t *testing.T) {
	deleteCalls := 0
	infoCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/clouddrive/file/delete":
			deleteCalls++
			writeDeleteJSON(w, http.StatusForbidden, Resp{
				Status: 403, Code: 403, Message: "permission denied",
			})
		case "/1/clouddrive/file":
			infoCalls++
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200, "code": 0, "data": map[string]any{"list": []any{}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	err := d.Remove(context.Background(), deleteTestObject("fid-3"))
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err=%v, want permission denied", err)
	}
	if deleteCalls != 1 || infoCalls != 0 {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want 1/0", deleteCalls, infoCalls)
	}
}

func TestRemoveReliableFailsClosedAfterRetryBudget(t *testing.T) {
	deleteCalls := 0
	infoCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/clouddrive/file/delete":
			deleteCalls++
			writeDeleteJSON(w, http.StatusInternalServerError, Resp{
				Status: 500, Code: 500, Message: "inner error, requestId persistent",
			})
		case "/1/clouddrive/file":
			infoCalls++
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{"list": []map[string]any{{
					"fid": "fid-4", "file_name": "chunk.bucket.4", "file": true,
				}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	err := d.Remove(context.Background(), deleteTestObject("fid-4"))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleteCalls != deleteControlMaxAttempts || infoCalls != deleteControlMaxAttempts {
		t.Fatalf("deleteCalls=%d infoCalls=%d, want %d/%d", deleteCalls, infoCalls,
			deleteControlMaxAttempts, deleteControlMaxAttempts)
	}
}

func TestRemoveReliableCanceledContextDoesNotSendRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeDeleteJSON(w, http.StatusOK, Resp{Status: 200, Code: 0})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.Remove(ctx, deleteTestObject("fid-5"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Fatalf("calls=%d, want 0", calls)
	}
}

func TestDeleteFileExistsByFIDRequiresExactFID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/1/clouddrive/file" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("fids"); got != "target-fid" {
			t.Fatalf("fids=%q, want target-fid", got)
		}
		writeDeleteJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data": map[string]any{"list": []map[string]any{{
				"fid": "different-fid", "file_name": "same-name", "file": true,
			}}},
		})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil {
		t.Fatalf("deleteFileExistsByFID: %v", err)
	}
	if exists {
		t.Fatal("different FID must not verify target existence")
	}
}

// deleteAlwaysTransientVerifierServer answers every delete with the observed
// transient provider error and delegates the verification query to verify.
func deleteAlwaysTransientVerifierServer(deleteCalls, infoCalls *int, verify http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/delete":
			*deleteCalls++
			writeDeleteJSON(w, http.StatusInternalServerError, Resp{
				Status: 500, Code: 500, Message: "inner error, requestId verify-guard",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file":
			*infoCalls++
			verify(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestRemoveReliableFailsClosedOnNonJSONVerifierResponse(t *testing.T) {
	verifiers := map[string]http.HandlerFunc{
		"html gateway 502": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
		},
		"html waf 403": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html><body>blocked</body></html>"))
		},
		"no content": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}
	for name, verify := range verifiers {
		t.Run(name, func(t *testing.T) {
			deleteCalls, infoCalls := 0, 0
			srv := httptest.NewServer(deleteAlwaysTransientVerifierServer(&deleteCalls, &infoCalls, verify))
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			if err := d.Remove(context.Background(), deleteTestObject("fid-nonjson")); err == nil {
				t.Fatal("Remove must not report success when verification is not well-formed")
			}
			if deleteCalls != deleteControlMaxAttempts || infoCalls != deleteControlMaxAttempts {
				t.Fatalf("deleteCalls=%d infoCalls=%d, want %d/%d", deleteCalls, infoCalls,
					deleteControlMaxAttempts, deleteControlMaxAttempts)
			}
		})
	}
}

func TestRemoveReliableFailsClosedOnRejectedVerifierEnvelope(t *testing.T) {
	envelopes := map[string]map[string]any{
		"non zero code": {
			"status": 200, "code": 31001, "message": "need login",
			"data": map[string]any{"list": []any{}},
		},
		"error status with http 200": {
			"status": 400, "code": 0, "message": "bad request",
			"data": map[string]any{"list": []any{}},
		},
	}
	for name, envelope := range envelopes {
		t.Run(name, func(t *testing.T) {
			deleteCalls, infoCalls := 0, 0
			srv := httptest.NewServer(deleteAlwaysTransientVerifierServer(&deleteCalls, &infoCalls,
				func(w http.ResponseWriter, r *http.Request) {
					writeDeleteJSON(w, http.StatusOK, envelope)
				}))
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			if err := d.Remove(context.Background(), deleteTestObject("fid-envelope")); err == nil {
				t.Fatal("Remove must not report success when the provider envelope is rejected")
			}
			if deleteCalls != deleteControlMaxAttempts || infoCalls != deleteControlMaxAttempts {
				t.Fatalf("deleteCalls=%d infoCalls=%d, want %d/%d", deleteCalls, infoCalls,
					deleteControlMaxAttempts, deleteControlMaxAttempts)
			}
		})
	}
}

func TestRemoveReliableFailsClosedWhenVerifierListIsMissing(t *testing.T) {
	bodies := map[string]map[string]any{
		"data missing": {"status": 200, "code": 0},
		"data null":    {"status": 200, "code": 0, "data": nil},
		"list missing": {"status": 200, "code": 0, "data": map[string]any{}},
		"list null":    {"status": 200, "code": 0, "data": map[string]any{"list": nil}},
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			deleteCalls, infoCalls := 0, 0
			srv := httptest.NewServer(deleteAlwaysTransientVerifierServer(&deleteCalls, &infoCalls,
				func(w http.ResponseWriter, r *http.Request) {
					writeDeleteJSON(w, http.StatusOK, body)
				}))
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			if err := d.Remove(context.Background(), deleteTestObject("fid-nolist")); err == nil {
				t.Fatal("Remove must not report success when the response carries no list")
			}
			if deleteCalls != deleteControlMaxAttempts || infoCalls != deleteControlMaxAttempts {
				t.Fatalf("deleteCalls=%d infoCalls=%d, want %d/%d", deleteCalls, infoCalls,
					deleteControlMaxAttempts, deleteControlMaxAttempts)
			}
		})
	}
}

func TestDeleteFileExistsByFIDTreatsExplicitEmptyListAsAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"list": []any{}},
		})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil {
		t.Fatalf("deleteFileExistsByFID: %v", err)
	}
	if exists {
		t.Fatal("an explicit empty list must report the FID as absent")
	}
}

func TestDeleteFileExistsByFIDDetectsPresentFID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data": map[string]any{"list": []map[string]any{
				{"fid": "other-fid", "file_name": "chunk.bucket.4", "file": true},
				{"fid": "target-fid", "file_name": "chunk.index.4", "file": true},
			}},
		})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil {
		t.Fatalf("deleteFileExistsByFID: %v", err)
	}
	if !exists {
		t.Fatal("a list containing the exact FID must report it as present")
	}
}
