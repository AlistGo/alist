package quark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
)

func TestUCDownloadToken(t *testing.T) {
	// Independently generated with OpenSSL AES-128-CBC and its default PKCS7 padding.
	const want = "NelUvDsfgQ5vcbK/BaZ1HoSnF9RePiGAuxFxhYc/dPslAA=="
	got, err := ucDownloadToken("0123456789abcdefghijklmn")
	if err != nil || got != want {
		t.Fatalf("token = %q, %v; want %q", got, err, want)
	}
	for _, id := range []string{"", strings.Repeat("x", 23), strings.Repeat("x", 25)} {
		if _, err := ucDownloadToken(id); err == nil {
			t.Fatalf("accepted UTDID length %d", len(id))
		}
	}
}

func TestUCDeviceIDPersistsAcrossInit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("ut") {
			t.Error("device token sent to config endpoint")
		}
		writeJSON(w, 200, Resp{Status: 200})
	}))
	defer srv.Close()
	d := newTestDriver(srv.URL)
	d.config.Name = "UC"
	d.AdditionVersion = 3
	if err := d.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer d.Drop(context.Background())
	id := d.UTDID
	if len(id) != 24 {
		t.Fatalf("device ID length = %d", len(id))
	}
	saved, err := db.GetStorageById(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := newTestDriver(srv.URL)
	restored.config.Name = "UC"
	if err := json.Unmarshal([]byte(saved.Addition), &restored.Addition); err != nil {
		t.Fatal(err)
	}
	if err := restored.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restored.Drop(context.Background())
	if restored.UTDID != id {
		t.Fatal("device identity changed after reload")
	}
	other := newTestDriver(srv.URL)
	other.config.Name = "UC"
	other.AdditionVersion = 3
	if err := other.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer other.Drop(context.Background())
	if other.UTDID == id {
		t.Fatal("different storages share device identity")
	}
}

func TestDownloadRequestDeviceToken(t *testing.T) {
	for _, name := range []string{"UC", "Quark"} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/1/clouddrive/file/download" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				q := r.URL.Query()
				if name == "UC" {
					if q.Get("ut") != "NelUvDsfgQ5vcbK/BaZ1HoSnF9RePiGAuxFxhYc/dPslAA==" {
						t.Errorf("incorrect or badly escaped ut: %q", q.Get("ut"))
					}
					if q.Get("pr") != "UCBrowser" {
						t.Errorf("pr = %q", q.Get("pr"))
					}
				} else if q.Has("ut") {
					t.Error("UC token leaked to Quark")
				}
				if q.Get("fr") != "pc" {
					t.Error("missing pc platform")
				}
				var body struct {
					Fids []string `json:"fids"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Fids) != 1 || body.Fids[0] != "f1" {
					t.Errorf("invalid download body: %+v, %v", body, err)
				}
				if r.Header.Get("Cookie") != "__puus=before" {
					t.Error("missing request cookie")
				}
				w.Header().Add("Set-Cookie", "__puus=after; Path=/")
				writeJSON(w, 200, map[string]interface{}{"status": 200, "code": 0, "data": []map[string]string{{"download_url": "https://cdn.example.com/file"}}})
			}))
			defer srv.Close()
			d := newTestDriver(srv.URL)
			d.config.Name = name
			d.UTDID = "0123456789abcdefghijklmn"
			if name == "UC" {
				d.conf.pr = "UCBrowser"
			}
			d.Cookie = "__puus=before"
			link, err := d.Link(context.Background(), &File{Fid: "f1"}, model.LinkArgs{})
			if err != nil {
				t.Fatal(err)
			}
			if link.URL != "https://cdn.example.com/file" || link.Header.Get("Cookie") != "__puus=before" {
				t.Fatalf("incorrect download link: %+v", link)
			}
			if !strings.Contains(d.Cookie, "__puus=after") {
				t.Error("cookie rotation lost")
			}
		})
	}
}

func TestDownloadEmptyResponse(t *testing.T) {
	for _, data := range []string{`[]`, `[{"download_url":""}]`} {
		t.Run(data, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":200,"code":0,"data":` + data + `}`))
			}))
			defer srv.Close()
			d := newTestDriver(srv.URL)
			if _, err := d.getDownloadLink(&File{Fid: "f1"}); err == nil {
				t.Fatal("expected missing link error")
			}
		})
	}
}
