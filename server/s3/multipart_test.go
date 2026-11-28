package s3

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "github.com/alist-org/alist/v3/drivers/local"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMultipartUpload(t *testing.T) {
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db.Init(database)

	root := t.TempDir()
	addition, err := json.Marshal(map[string]string{"root_folder_path": root})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, err := op.CreateStorage(ctx, model.Storage{
		Driver: "Local", MountPath: "/s3-multipart-test", Addition: string(addition),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := op.DeleteStorageById(ctx, id); err != nil {
			t.Error(err)
		}
	})
	if err := op.SaveSettingItem(&model.SettingItem{
		Key: conf.S3Buckets, Value: `[{"name":"multipart","path":"/s3-multipart-test"}]`,
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := NewServer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	request := func(method, path, body string, wantStatus int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Length", strconv.Itoa(len(body)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != wantStatus {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, r.URL.Path, w.Code, wantStatus, w.Body)
		}
		return w
	}

	const key = "分片 + &.mp4"
	path := "/multipart/" + url.PathEscape(key)
	w := request(http.MethodPost, path+"?uploads=", "", http.StatusOK)
	var initiated struct {
		XMLName xml.Name `xml:"InitiateMultipartUploadResult"`
		Bucket  string
		Key     string
		ID      string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &initiated); err != nil {
		t.Fatal(err)
	}
	if initiated.Bucket != "multipart" || initiated.Key != key || initiated.ID == "" {
		t.Fatalf("invalid response: %s", w.Body)
	}

	const payload = "multipart contents"
	query := "?" + url.Values{"uploadId": {initiated.ID}}.Encode()
	part := request(http.MethodPut, path+query+"&partNumber=1", payload, http.StatusOK)
	etag := part.Header().Get("ETag")
	if etag == "" {
		t.Fatal("UploadPart returned no ETag")
	}
	complete := fmt.Sprintf("<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>%s</ETag></Part></CompleteMultipartUpload>", etag)
	w = request(http.MethodPost, path+query, complete, http.StatusOK)
	var completed struct {
		XMLName xml.Name `xml:"CompleteMultipartUploadResult"`
		ETag    string
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &completed); err != nil || completed.ETag == "" {
		t.Fatalf("complete response: %v, %s", err, w.Body)
	}
	got, err := os.ReadFile(filepath.Join(root, key))
	if err != nil || string(got) != payload {
		t.Fatalf("final object: %q, %v", got, err)
	}

	w = request(http.MethodPost, "/nonexistent-multipart-bucket/object?uploads=", "", http.StatusNotFound)
	var failure struct {
		XMLName xml.Name `xml:"Error"`
		Code    string
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &failure); err != nil || failure.Code != "NoSuchBucket" {
		t.Fatalf("init failure response: %v, %s", err, w.Body)
	}
}
