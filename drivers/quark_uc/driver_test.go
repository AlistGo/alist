package quark

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
)

func TestMakeDirWaitsUntilDirectoryIsVisible(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file":
			writeJSON(w, http.StatusOK, Resp{Status: 200, Code: 0, Message: "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			if listCalls == 1 {
				writeJSON(w, http.StatusOK, map[string]any{
					"status": 200,
					"code":   0,
					"data":   map[string]any{"list": []any{}},
					"metadata": map[string]any{
						"_size": 100, "_page": 1, "_count": 0, "_total": 0,
					},
				})
				return
			}
			writeDirectoryList(w, "dir-1", "Pool")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: "parent", Name: "parent", IsFolder: true}
	obj, err := d.MakeDir(context.Background(), parent, "Pool")
	if err != nil {
		t.Fatalf("MakeDir: %v", err)
	}
	if obj == nil || obj.GetID() != "dir-1" || obj.GetName() != "Pool" || !obj.IsDir() {
		t.Fatalf("MakeDir object = %#v, want visible directory dir-1/Pool", obj)
	}
	if listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", listCalls)
	}
}

func TestMakeDirKeepsSuccessfulCreateWhenVisibilityChecksAllError(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file":
			writeJSON(w, http.StatusOK, Resp{Status: 200, Code: 0, Message: "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			writeJSON(w, http.StatusInternalServerError, Resp{Status: http.StatusInternalServerError, Code: 500, Message: "temporary list failure"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: "parent", Name: "parent", IsFolder: true}
	obj, err := d.MakeDir(context.Background(), parent, "Pool")
	if err != nil {
		t.Fatalf("MakeDir should preserve successful create: %v", err)
	}
	if obj != nil {
		t.Fatalf("MakeDir object = %#v, want nil so op.MakeDir clears parent cache", obj)
	}
	if listCalls != mkdirVisibilityAttempts {
		t.Fatalf("list calls = %d, want %d", listCalls, mkdirVisibilityAttempts)
	}
}

func TestMakeDirKeepsSuccessfulCreateWhenCleanListingsRemainStale(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file":
			writeJSON(w, http.StatusOK, Resp{Status: 200, Code: 0, Message: "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": []any{}},
				"metadata": map[string]any{
					"_size": 100, "_page": 1, "_count": 0, "_total": 0,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: "parent", Name: "parent", IsFolder: true}
	obj, err := d.MakeDir(context.Background(), parent, "Pool")
	if err != nil {
		t.Fatalf("MakeDir should preserve successful create despite stale listings: %v", err)
	}
	if obj != nil {
		t.Fatalf("MakeDir object = %#v, want nil so op.MakeDir clears parent cache", obj)
	}
	if listCalls != mkdirVisibilityAttempts {
		t.Fatalf("list calls = %d, want %d", listCalls, mkdirVisibilityAttempts)
	}
}

func TestMakeDirRecoversSameNameConflictAfterVisibilityCheck(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file":
			writeJSON(w, http.StatusConflict, Resp{Status: http.StatusConflict, Code: 32003, Message: mkdirConflictMessage})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			writeDirectoryList(w, "dir-existing", "Pool")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: "parent", Name: "parent", IsFolder: true}
	obj, err := d.MakeDir(context.Background(), parent, "Pool")
	if err != nil {
		t.Fatalf("MakeDir should recover verified same-name conflict: %v", err)
	}
	if obj == nil || obj.GetID() != "dir-existing" || obj.GetName() != "Pool" || !obj.IsDir() {
		t.Fatalf("MakeDir object = %#v, want verified existing directory", obj)
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", listCalls)
	}
}

func TestMakeDirPreservesConflictWhenDirectoryNeverAppears(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file":
			writeJSON(w, http.StatusConflict, Resp{Status: http.StatusConflict, Code: 32003, Message: mkdirConflictMessage})
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": []any{}},
				"metadata": map[string]any{
					"_size": 100, "_page": 1, "_count": 0, "_total": 0,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: "parent", Name: "parent", IsFolder: true}
	obj, err := d.MakeDir(context.Background(), parent, "Pool")
	if obj != nil {
		t.Fatalf("MakeDir object = %#v, want nil on unverified conflict", obj)
	}
	if err == nil || err.Error() != mkdirConflictMessage {
		t.Fatalf("MakeDir error = %v, want %q", err, mkdirConflictMessage)
	}
	if listCalls != mkdirVisibilityAttempts {
		t.Fatalf("list calls = %d, want %d", listCalls, mkdirVisibilityAttempts)
	}
}

func TestWaitForDirVisibleHonorsCanceledContext(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	obj, _, err := d.waitForDirVisible(ctx, "parent", "Pool")
	if obj != nil {
		t.Fatalf("waitForDirVisible object = %#v, want nil", obj)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForDirVisible error = %v, want context.Canceled", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0 for pre-canceled context", requests)
	}
}

func TestFindDirRejectsSameNameFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/1/clouddrive/file/sort" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data": map[string]any{"list": []map[string]any{{
				"fid": "file-1", "file_name": "Pool", "file": true,
			}}},
			"metadata": map[string]any{
				"_size": 100, "_page": 1, "_count": 1, "_total": 1,
			},
		})
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	obj, err := d.findDir(context.Background(), "parent", "Pool")
	if err != nil {
		t.Fatalf("findDir: %v", err)
	}
	if obj != nil {
		t.Fatalf("findDir accepted a same-name file as a directory: %#v", obj)
	}
}

func TestFindDirPaginatesWithoutTotalAndUnescapesName(t *testing.T) {
	listCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/1/clouddrive/file/sort" {
			http.NotFound(w, r)
			return
		}
		listCalls++
		switch r.URL.Query().Get("_page") {
		case "1":
			list := make([]map[string]any, 100)
			for i := range list {
				list[i] = map[string]any{
					"fid": "other", "file_name": "Other", "file": false,
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": list},
				"metadata": map[string]any{
					"_size": 100, "_page": 1, "_count": 100, "_total": 0,
				},
			})
		case "2":
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{"list": []map[string]any{{
					"fid": "dir-target", "file_name": "A&amp;B", "file": false,
				}}},
				"metadata": map[string]any{
					"_size": 100, "_page": 2, "_count": 1, "_total": 0,
				},
			})
		default:
			t.Fatalf("unexpected page: %s", r.URL.Query().Get("_page"))
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	obj, err := d.findDir(context.Background(), "parent", "A&B")
	if err != nil {
		t.Fatalf("findDir: %v", err)
	}
	if obj == nil || obj.GetID() != "dir-target" || obj.GetName() != "A&B" || !obj.IsDir() {
		t.Fatalf("findDir object = %#v, want decoded directory dir-target/A&B", obj)
	}
	if listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", listCalls)
	}
}

func writeDirectoryList(w http.ResponseWriter, fid, name string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": 200,
		"code":   0,
		"data": map[string]any{"list": []map[string]any{{
			"fid": fid, "file_name": name, "file": false,
		}}},
		"metadata": map[string]any{
			"_size": 100, "_page": 1, "_count": 1, "_total": 1,
		},
	})
}
