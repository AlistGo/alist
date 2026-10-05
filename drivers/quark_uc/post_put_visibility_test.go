package quark

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	streamPkg "github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
)

var _ driver.PutResult = (*QuarkOrUC)(nil)

func uploadVisibilityTestObj(name string, payload []byte) *model.Object {
	return &model.Object{
		Name: name,
		Size: int64(len(payload)),
		HashInfo: utils.NewHashInfoByMap(map[*utils.HashType]string{
			utils.MD5:  utils.HashData(utils.MD5, payload),
			utils.SHA1: utils.HashData(utils.SHA1, payload),
		}),
	}
}

func TestPutReturnsPreallocatedFIDForImmediateFollowupOperations(t *testing.T) {
	const (
		parentID = "parent-1"
		fid      = "fid-lock-1"
		name     = "lock_keep_alive.@writer_version_0.test"
	)
	payload := []byte("lock")

	preCalls := 0
	hashCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/upload/pre":
			preCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{
					"task_id": "task-1",
					"fid":     fid,
				},
				"metadata": map[string]any{
					"part_size": 4 * 1024 * 1024,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/update/hash":
			hashCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{
					"finish": true,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	parent := &model.Object{ID: parentID, Name: "parent", IsFolder: true}
	fs := &streamPkg.FileStream{
		Obj:      uploadVisibilityTestObj(name, payload),
		Reader:   bytes.NewReader(payload),
		Mimetype: "application/octet-stream",
	}
	defer fs.Close()

	obj, err := d.Put(context.Background(), parent, fs, func(float64) {})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if obj == nil {
		t.Fatal("Put returned nil object; op.Put cannot seed the parent cache")
	}
	if obj.GetID() != fid {
		t.Fatalf("Put object fid=%q, want %q", obj.GetID(), fid)
	}
	if obj.GetName() != name {
		t.Fatalf("Put object name=%q, want %q", obj.GetName(), name)
	}
	if obj.GetSize() != int64(len(payload)) {
		t.Fatalf("Put object size=%d, want %d", obj.GetSize(), len(payload))
	}
	if obj.IsDir() {
		t.Fatal("Put object is a directory, want file")
	}
	if preCalls != 1 || hashCalls != 1 {
		t.Fatalf("preCalls=%d hashCalls=%d, want 1/1", preCalls, hashCalls)
	}
}

func TestUploadedFileFromPreDoesNotInventMissingFID(t *testing.T) {
	fs := &streamPkg.FileStream{
		Obj:    &model.Object{Name: "lock", Size: 1},
		Reader: bytes.NewReader([]byte{1}),
	}
	if obj := uploadedFileFromPre(UpPreResp{}, fs); obj != nil {
		t.Fatalf("uploadedFileFromPre without fid = %#v, want nil", obj)
	}
}

func TestPutResultBridgesStaleListingForImmediateRemove(t *testing.T) {
	const (
		parentID = "parent-lock"
		fid      = "fid-new-lock"
		name     = "lock_keep_alive.@writer_version_0.regression"
	)
	payload := []byte("lock")

	listCalls := 0
	preCalls := 0
	hashCalls := 0
	deleteCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1/clouddrive/file/sort":
			listCalls++
			// Model the production failure with the strongest edge case: the
			// backend listing remains empty/stale, so there is no parent list cache
			// entry for addCacheObj to update after the upload.
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"list": []any{}},
				"metadata": map[string]any{
					"_size": 100, "_page": 1, "_count": 0, "_total": 0,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/upload/pre":
			preCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data": map[string]any{
					"task_id":    "task-lock",
					"fid":        fid,
					"upload_id":  "upload-lock",
					"obj_key":    "object-lock",
					"upload_url": "https://oss.test",
					"bucket":     "bucket",
					"auth_info":  "auth",
				},
				"metadata": map[string]any{
					"part_size": 4 * 1024 * 1024,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/update/hash":
			hashCalls++
			writeJSON(w, http.StatusOK, map[string]any{
				"status": 200,
				"code":   0,
				"data":   map[string]any{"finish": true},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/1/clouddrive/file/delete":
			deleteCalls++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read delete body: %v", err)
			}
			if !bytes.Contains(body, []byte(fid)) {
				t.Fatalf("delete body %q does not contain uploaded fid %q", body, fid)
			}
			writeJSON(w, http.StatusOK, Resp{Status: 200, Code: 0, Message: "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.RootFolderID = parentID
	fs := &streamPkg.FileStream{
		Obj:      uploadVisibilityTestObj(name, payload),
		Reader:   bytes.NewReader(payload),
		Mimetype: "application/octet-stream",
	}

	ctx := context.Background()
	if err := op.Put(ctx, d, "/", fs, nil); err != nil {
		t.Fatalf("op.Put: %v", err)
	}
	if err := op.Remove(ctx, d, "/"+name); err != nil {
		t.Fatalf("op.Remove after successful PutResult: %v", err)
	}

	if listCalls != 2 {
		t.Fatalf("listCalls=%d, want 2; delete should survive a second stale backend lookup", listCalls)
	}
	if preCalls != 1 || hashCalls != 1 || deleteCalls != 1 {
		t.Fatalf("pre/hash/delete calls=%d/%d/%d, want 1/1/1", preCalls, hashCalls, deleteCalls)
	}

	// Successful removal must invalidate the short consistency bridge. With the
	// backend still modeled as empty/stale, the removed path must not reappear.
	if _, err := op.Get(ctx, d, "/"+name); !errs.IsObjectNotFound(err) {
		t.Fatalf("op.Get after remove error=%v, want object not found", err)
	}
	if listCalls != 3 {
		t.Fatalf("listCalls=%d after removal verification, want 3", listCalls)
	}
}
