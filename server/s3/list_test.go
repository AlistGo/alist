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
	"testing"
	"time"

	_ "github.com/alist-org/alist/v3/drivers/local"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListObjectsDirectoryMarkerLastModified(t *testing.T) {
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
	for _, dir := range []string{"photos", "empty"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "photos", "photo.jpg"), []byte("photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	addition, err := json.Marshal(map[string]string{"root_folder_path": root})
	if err != nil {
		t.Fatal(err)
	}
	id, err := op.CreateStorage(context.Background(), model.Storage{
		Driver: "Local", MountPath: "/s3-list-test", Addition: string(addition),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := op.DeleteStorageById(context.Background(), id); err != nil {
			t.Error(err)
		}
	})
	if err := op.SaveSettingItem(&model.SettingItem{
		Key: conf.S3Buckets, Value: `[{"name":"photos","path":"/s3-list-test"}]`,
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := NewServer(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, version := range []string{"1", "2"} {
		for _, prefix := range []string{"photos/", "empty/", "missing/", "photos/photo"} {
			for _, delimiter := range []string{"", "/"} {
				t.Run(fmt.Sprintf("v%s/%s/delimiter=%s", version, prefix, delimiter), func(t *testing.T) {
					query := url.Values{"prefix": {prefix}, "max-keys": {"1000"}}
					if version == "2" {
						query.Set("list-type", "2")
					}
					if delimiter != "" {
						query.Set("delimiter", delimiter)
					}
					r := httptest.NewRequest(http.MethodGet, "/photos?"+query.Encode(), nil)
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, r)
					if w.Code != http.StatusOK {
						t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
					}
					var result struct {
						Contents []struct {
							Key          string
							LastModified string
							Size         int64
						}
					}
					if err := xml.Unmarshal(w.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					want := 1
					if prefix == "photos/" {
						want = 2
					}
					if len(result.Contents) != want {
						t.Fatalf("got %d objects, want %d: %s", len(result.Contents), want, w.Body.String())
					}
					seen := make(map[string]bool)
					for _, item := range result.Contents {
						if seen[item.Key] {
							t.Fatalf("duplicate key %q", item.Key)
						}
						seen[item.Key] = true
						modified, err := time.Parse(time.RFC3339Nano, item.LastModified)
						if err != nil || modified.IsZero() {
							t.Errorf("key %q has missing or invalid LastModified %q", item.Key, item.LastModified)
						}
						if item.Key == "photos/photo.jpg" && item.Size != 5 {
							t.Errorf("photo size=%d, want 5", item.Size)
						}
					}
				})
			}
		}
	}
}
