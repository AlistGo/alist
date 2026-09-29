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

const (
	fileInfoPath   = "/1/clouddrive/file/info"
	fileDeletePath = "/1/clouddrive/file/delete"
	legacyFilePath = "/1/clouddrive/file"
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

// fileInfoPresent is the real-provider PRESENT shape: HTTP 200, provider
// status 200, code 0, and a data object naming the requested FID.
func fileInfoPresent(fid string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data":   map[string]any{"fid": fid, "file_name": "chunk.bucket.4", "file": true},
		})
	}
}

// fileInfoNotFound is the real-provider ABSENT signature: HTTP 404, provider
// status 404, code 21001, no data object.
func fileInfoNotFound() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusNotFound, map[string]any{
			"status":  404,
			"code":    21001,
			"message": "file not found [xxxxxxxx]",
		})
	}
}

// deleteWithVerifier answers every delete with the observed transient provider
// error and delegates the verification query to verify.
func deleteWithVerifier(deleteCalls, infoCalls *int, verify http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			*deleteCalls++
			writeDeleteJSON(w, http.StatusInternalServerError, Resp{
				Status: 500, Code: 500, Message: "inner error, requestId verify-guard",
			})
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			*infoCalls++
			verify(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestIsRetryableQuarkDeleteError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "observed provider error", err: errors.New("inner error, requestId 95sg27-abc"), want: true},
		{name: "case insensitive", err: errors.New("Inner Error: RequestId abc"), want: true},
		{name: "provider error type", err: &providerError{HTTPStatus: 500, Status: 500, Code: 500, Message: "inner error, requestId abc"}, want: true},
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

// ---------------------------------------------------------------------------
// Verifier oracle: wire contract
// ---------------------------------------------------------------------------

func TestDeleteFileExistsByFIDUsesFileInfoEndpoint(t *testing.T) {
	var (
		gotPath    string
		gotFid     string
		gotFids    string
		gotMethod  string
		legacyHits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == legacyFilePath {
			// The disproven oracle. Answer it in the shape it used to return so
			// that a regression would look "successful" rather than crash.
			legacyHits++
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200, "code": 0,
				"data": map[string]any{"list": []map[string]any{{"fid": "target-fid"}}},
			})
			return
		}
		gotPath, gotMethod = r.URL.Path, r.Method
		gotFid = r.URL.Query().Get("fid")
		gotFids = r.URL.Query().Get("fids")
		fileInfoPresent("target-fid")(w, r)
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v, want true/nil", exists, err)
	}
	if gotMethod != http.MethodGet || gotPath != fileInfoPath {
		t.Fatalf("method=%s path=%s, want GET %s", gotMethod, gotPath, fileInfoPath)
	}
	if gotFid != "target-fid" {
		t.Fatalf("fid=%q, want target-fid", gotFid)
	}
	if gotFids != "" {
		t.Fatalf("legacy fids param was sent: %q", gotFids)
	}
	if legacyHits != 0 {
		t.Fatalf("legacy /file?fids oracle was queried %d times, want 0", legacyHits)
	}
}

// A. present
func TestDeleteFileExistsByFIDPresent(t *testing.T) {
	srv := httptest.NewServer(fileInfoPresent("target-fid"))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil {
		t.Fatalf("deleteFileExistsByFID: %v", err)
	}
	if !exists {
		t.Fatal("a successful envelope naming the requested FID must report present")
	}
}

// B. real-provider absent signature
func TestDeleteFileExistsByFIDRealAbsentSignature(t *testing.T) {
	srv := httptest.NewServer(fileInfoNotFound())
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err != nil {
		t.Fatalf("the proven not-found signature must not surface an error: %v", err)
	}
	if exists {
		t.Fatal("the proven not-found signature must report absent")
	}
}

// C/D/E/F/G/H/I and every other non-absence shape.
func TestDeleteFileExistsByFIDFailsClosedOnEverythingElse(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"C invalid fid 14001", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusBadRequest, map[string]any{
				"status": 400, "code": 14001, "message": "Bad Parameter: [fid is invalid]",
			})
		}},
		{"D unauthorized 401", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusUnauthorized, map[string]any{
				"status": 401, "code": 31001, "message": "need login",
			})
		}},
		{"D forbidden 403", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusForbidden, map[string]any{
				"status": 403, "code": 31002, "message": "permission denied",
			})
		}},
		{"E rate limited 429", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusTooManyRequests, map[string]any{
				"status": 429, "code": 32003, "message": "too many requests",
			})
		}},
		{"F server error 500", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusInternalServerError, map[string]any{
				"status": 500, "code": 500, "message": "inner error, requestId abc",
			})
		}},
		{"F bad gateway 502 html", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html><body>502 Bad Gateway</body></html>"))
		}},
		{"G malformed json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":200,"code":0,"data":{`))
		}},
		{"H data missing", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{"status": 200, "code": 0})
		}},
		{"H data null", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{"status": 200, "code": 0, "data": nil})
		}},
		{"I mismatched fid", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200, "code": 0,
				"data": map[string]any{"fid": "different-fid", "file_name": "same-name"},
			})
		}},
		{"I empty fid in data", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200, "code": 0, "data": map[string]any{"file_name": "no-fid"},
			})
		}},
		{"success envelope with non-zero code", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200, "code": 31001, "message": "need login",
				"data": map[string]any{"fid": "target-fid"},
			})
		}},
		{"error status delivered with http 200", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 400, "code": 0, "message": "bad request",
				"data": map[string]any{"fid": "target-fid"},
			})
		}},
		{"missing provider code", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"data":   map[string]any{"fid": "target-fid"},
			})
		}},
		{"missing provider status", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"code": 0,
				"data": map[string]any{"fid": "target-fid"},
			})
		}},
		{"missing provider status and code", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{"fid": "target-fid"},
			})
		}},
		{"explicit zero provider status", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{
				"status": 0, "code": 0,
				"data": map[string]any{"fid": "target-fid"},
			})
		}},
		{"404 without provider envelope", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html><body>404</body></html>"))
		}},
		{"404 with unknown provider code", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusNotFound, map[string]any{
				"status": 404, "code": 29999, "message": "something else",
			})
		}},
		{"not-found code without 404 http status", func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusBadRequest, map[string]any{
				"status": 404, "code": 21001, "message": "file not found",
			})
		}},
		{"no content", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
			if err == nil {
				t.Fatalf("want a verification error, got exists=%v err=nil", exists)
			}
			if exists {
				t.Fatal("a failed verification must never report present")
			}
		})
	}
}

func TestDeleteFileExistsByFIDRejectsEmptyFID(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fileInfoPresent("")(w, r)
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "")
	if err == nil {
		t.Fatal("an empty fid must be rejected")
	}
	if exists {
		t.Fatal("an empty fid must never report present")
	}
	if calls != 0 {
		t.Fatalf("calls=%d, want 0", calls)
	}
}

// Regression proof: the disproven oracle's response shape cannot satisfy the
// new PRESENT contract even when it names the requested FID.
func TestDeleteFileExistsByFIDRejectsLegacyFidsListShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDeleteJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data": map[string]any{"list": []map[string]any{
				{"fid": "target-fid", "file_name": "chunk.bucket.4", "file": true},
			}},
		})
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	exists, err := d.deleteFileExistsByFID(context.Background(), "target-fid")
	if err == nil {
		t.Fatal("a legacy fids-list body must not satisfy the PRESENT contract")
	}
	if exists {
		t.Fatal("a legacy fids-list body must never report present")
	}
}

func TestIsQuarkFileNotFoundRequiresFullSignature(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"exact signature", &providerError{HTTPStatus: 404, Status: 404, Code: 21001}, true},
		{"wrong http status", &providerError{HTTPStatus: 400, Status: 404, Code: 21001}, false},
		{"wrong provider status", &providerError{HTTPStatus: 404, Status: 400, Code: 21001}, false},
		{"wrong code", &providerError{HTTPStatus: 404, Status: 404, Code: 14001}, false},
		{"invalid fid", &providerError{HTTPStatus: 400, Status: 400, Code: 14001}, false},
		{"plain error with matching text", errors.New("file not found [abc]"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isQuarkFileNotFound(tt.err); got != tt.want {
				t.Fatalf("isQuarkFileNotFound = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Remove integration
// ---------------------------------------------------------------------------

func TestRemoveReliableRetriesTransientWhenFIDStillExists(t *testing.T) {
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			deleteCalls++
			if deleteCalls == 1 {
				writeDeleteJSON(w, http.StatusInternalServerError, Resp{
					Status: 500, Code: 500, Message: "inner error, requestId delete-1",
				})
				return
			}
			writeDeleteJSON(w, http.StatusOK, Resp{Status: 200, Code: 0})
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			infoCalls++
			fileInfoPresent("fid-1")(w, r)
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
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(deleteWithVerifier(&deleteCalls, &infoCalls, fileInfoNotFound()))
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
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
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
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			infoCalls++
			if infoCalls == 1 {
				fileInfoPresent("fid-2")(w, r)
				return
			}
			fileInfoNotFound()(w, r)
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
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case fileDeletePath:
			deleteCalls++
			writeDeleteJSON(w, http.StatusForbidden, Resp{
				Status: 403, Code: 403, Message: "permission denied",
			})
		case fileInfoPath:
			infoCalls++
			fileInfoNotFound()(w, r)
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
	deleteCalls, infoCalls := 0, 0
	srv := httptest.NewServer(deleteWithVerifier(&deleteCalls, &infoCalls, fileInfoPresent("fid-4")))
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

func TestRemoveReliableFailsClosedWhenVerificationErrors(t *testing.T) {
	verifiers := map[string]http.HandlerFunc{
		"invalid fid 14001": func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusBadRequest, map[string]any{
				"status": 400, "code": 14001, "message": "Bad Parameter: [fid is invalid]",
			})
		},
		"rate limited 429": func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusTooManyRequests, map[string]any{
				"status": 429, "code": 32003, "message": "too many requests",
			})
		},
		"gateway html 502": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html><body>502</body></html>"))
		},
		"data missing": func(w http.ResponseWriter, r *http.Request) {
			writeDeleteJSON(w, http.StatusOK, map[string]any{"status": 200, "code": 0})
		},
	}
	for name, verify := range verifiers {
		t.Run(name, func(t *testing.T) {
			deleteCalls, infoCalls := 0, 0
			srv := httptest.NewServer(deleteWithVerifier(&deleteCalls, &infoCalls, verify))
			defer srv.Close()

			d := newDeleteTestDriver(srv.URL)
			if err := d.Remove(context.Background(), deleteTestObject("fid-verify")); err == nil {
				t.Fatal("Remove must not report success when verification fails")
			}
			if deleteCalls != deleteControlMaxAttempts || infoCalls != deleteControlMaxAttempts {
				t.Fatalf("deleteCalls=%d infoCalls=%d, want %d/%d", deleteCalls, infoCalls,
					deleteControlMaxAttempts, deleteControlMaxAttempts)
			}
		})
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
