package baidu_netdisk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type rewriteTransport struct{ target *url.URL }

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme, clone.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestInitRefreshesExpiredAccessToken(t *testing.T) {
	oldConfig := conf.Conf
	conf.Conf = conf.DefaultConfig()
	t.Cleanup(func() { conf.Conf = oldConfig })
	for _, code := range []int{20016, 111, -6} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := database.DB()
			t.Cleanup(func() { sqlDB.Close() })
			if err = database.AutoMigrate(&model.Storage{}); err != nil {
				t.Fatal(err)
			}
			db.Init(database)
			calls, refreshes := 0, 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/rest/2.0/xpan/nas":
					calls++
					if calls == 1 {
						fmt.Fprintf(w, `{"errno":%d}`, code)
						return
					}
					if r.URL.Query().Get("access_token") != "new-access" {
						t.Error("retry did not use refreshed access token")
					}
					fmt.Fprint(w, `{"errno":0,"vip_type":2}`)
				case "/oauth/2.0/token":
					refreshes++
					q := r.URL.Query()
					if q.Get("grant_type") != "refresh_token" || q.Get("refresh_token") != "old-refresh" || q.Get("client_id") != "test-client" || q.Get("client_secret") != "test-secret" {
						t.Error("incorrect refresh handshake")
					}
					fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh"}`)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			target, _ := url.Parse(srv.URL)
			old := base.RestyClient
			base.RestyClient = resty.New().SetTransport(rewriteTransport{target})
			t.Cleanup(func() { base.RestyClient = old })
			d := &BaiduNetdisk{Addition: Addition{AccessToken: "old-access", RefreshToken: "old-refresh", ClientID: "test-client", ClientSecret: "test-secret"}}
			if err = d.Init(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 2 || refreshes != 1 || d.vipType != 2 {
				t.Fatalf("calls=%d refreshes=%d vip=%d", calls, refreshes, d.vipType)
			}
			saved, err := db.GetStorageById(d.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(saved.Addition, `"refresh_token":"new-refresh"`) {
				t.Fatal("rotated refresh token not persisted")
			}
		})
	}
}

func TestRefreshFailureStopsRetry(t *testing.T) {
	calls, refreshes := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/2.0/token" {
			refreshes++
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token revoked"}`)
			return
		}
		calls++
		fmt.Fprint(w, `{"errno":20016}`)
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	old := base.RestyClient
	base.RestyClient = resty.New().SetTransport(rewriteTransport{target})
	defer func() { base.RestyClient = old }()
	d := &BaiduNetdisk{}
	_, err := d.get("/xpan/as", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 || refreshes != 1 {
		t.Fatalf("calls=%d refreshes=%d", calls, refreshes)
	}
}

func TestOtherErrorsDoNotRefreshToken(t *testing.T) {
	calls, refreshes := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/2.0/token" {
			refreshes++
		}
		calls++
		fmt.Fprint(w, `{"errno":20013}`)
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	old := base.RestyClient
	base.RestyClient = resty.New().SetTransport(rewriteTransport{target})
	defer func() { base.RestyClient = old }()
	d := &BaiduNetdisk{}
	_, err := d.get("/xpan/as", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "20013") || refreshes != 0 || calls != 3 {
		t.Fatalf("err=%v calls=%d refreshes=%d", err, calls, refreshes)
	}
}
