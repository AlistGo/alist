package quark

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/cookie"
	"github.com/go-resty/resty/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	// 初始化内存数据库，保证 op.MustSaveDriverStorage 在测试中安全执行
	conf.Conf = conf.DefaultConfig()
	d, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.Init(d)
	os.Exit(m.Run())
}

var testDriverSeq int

func newTestDriver(srvURL string) *QuarkOrUC {
	testDriverSeq++
	d := &QuarkOrUC{Storage: model.Storage{MountPath: fmt.Sprintf("test-%d", testDriverSeq)}}
	d.conf = Conf{
		ua:      "test-ua",
		referer: "https://pan.quark.cn",
		api:     srvURL + "/1/clouddrive",
		pr:      "ucpro",
	}
	d.client = resty.New()
	return d
}

// recordHandler 记录收到的请求路径并转发给 handler
type recordHandler struct {
	mu      sync.Mutex
	paths   []string
	handler http.HandlerFunc
}

func (r *recordHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.paths = append(r.paths, req.URL.Path)
	r.mu.Unlock()
	r.handler(w, req)
}

func (r *recordHandler) Paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.paths...)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func TestGetFilesPreservesThumbnail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/sort" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": 200,
			"code":   0,
			"data": map[string]any{
				"list": []map[string]any{{
					"fid":       "episode-1",
					"file_name": "episode.mp4",
					"file":      true,
					"thumbnail": "https://example.com/episode.jpg",
				}},
			},
			"metadata": map[string]any{"_total": 1},
		})
	}))
	defer server.Close()

	files, err := newTestDriver(server.URL).GetFiles("0")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("file count = %d, want 1", len(files))
	}
	thumbnail, ok := model.GetThumb(files[0])
	if !ok {
		t.Fatal("listed file does not expose a thumbnail")
	}
	if want := "https://example.com/episode.jpg"; thumbnail != want {
		t.Fatalf("thumbnail = %q, want %q", thumbnail, want)
	}
}

func TestRefreshPuus(t *testing.T) {
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/config" {
			http.NotFound(w, r)
			return
		}
		if c := r.Header.Get("Cookie"); strings.Contains(c, "__puus") {
			t.Errorf("refresh request must not carry __puus, got cookie: %q", c)
		}
		w.Header().Add("Set-Cookie", "__puus=newpuus; Path=/")
		writeJSON(w, 200, Resp{Status: 200, Code: 0, Message: "ok"})
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "a=1; __puus=oldpuus; b=2"

	if err := d.refreshPuus(); err != nil {
		t.Fatalf("refreshPuus: %v", err)
	}
	if !strings.Contains(d.Cookie, "__puus=newpuus") {
		t.Fatalf("cookie not refreshed: %q", d.Cookie)
	}
	if !strings.Contains(d.Cookie, "a=1") || !strings.Contains(d.Cookie, "b=2") {
		t.Fatalf("refresh dropped unrelated cookies: %q", d.Cookie)
	}
	if len(rec.Paths()) != 1 || rec.Paths()[0] != "/1/clouddrive/config" {
		t.Fatalf("unexpected requests: %v", rec.Paths())
	}
}

func TestRefreshPuusRestoresCookieOnError(t *testing.T) {
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 500, Resp{Status: 500, Code: 0, Message: "boom"})
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "a=1; __puus=oldpuus; b=2"

	if err := d.refreshPuus(); err == nil {
		t.Fatal("want error from refreshPuus")
	}
	if d.Cookie != "a=1; __puus=oldpuus; b=2" {
		t.Fatalf("cookie not restored after error: %q", d.Cookie)
	}
}

func TestRefreshPuusRestoresCookieWhenServerDoesNotReissue(t *testing.T) {
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		// 服务端没有下发新的 __puus
		writeJSON(w, 200, Resp{Status: 200, Code: 0, Message: "ok"})
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "a=1; __puus=oldpuus; b=2"

	if err := d.refreshPuus(); err != nil {
		t.Fatalf("refreshPuus: %v", err)
	}
	if d.Cookie != "a=1; __puus=oldpuus; b=2" {
		t.Fatalf("cookie should be restored when server does not reissue: %q", d.Cookie)
	}
}

func TestRefreshPuusWithEmptyCookie(t *testing.T) {
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		if c := r.Header.Get("Cookie"); c != "" {
			t.Errorf("request cookie should be empty, got %q", c)
		}
		w.Header().Add("Set-Cookie", "__puus=newpuus; Path=/")
		writeJSON(w, 200, Resp{Status: 200, Code: 0, Message: "ok"})
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = ""

	if err := d.refreshPuus(); err != nil {
		t.Fatalf("refreshPuus: %v", err)
	}
	if !strings.Contains(d.Cookie, "__puus=newpuus") {
		t.Fatalf("cookie not refreshed: %q", d.Cookie)
	}
}

func TestGetDownloadLinkUsesRequestTimeCookie(t *testing.T) {
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/clouddrive/file/download":
			// 响应轮换 __puus，但下载 URL 签名基于请求时携带的 cookie
			w.Header().Add("Set-Cookie", "__puus=rotated; Path=/")
			writeJSON(w, 200, map[string]interface{}{
				"status": 200, "code": 0, "message": "ok",
				"data": []map[string]string{{"download_url": "https://cdn.example.com/f?sign=1"}},
			})
		default:
			http.NotFound(w, r)
		}
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "__puus=pre"

	link, err := d.getDownloadLink(&File{Fid: "f1"})
	if err != nil {
		t.Fatalf("getDownloadLink: %v", err)
	}
	// 下载请求头必须使用生成签名时的 cookie（快照），而不是响应更新后的值
	if got := link.Header.Get("Cookie"); got != "__puus=pre" {
		t.Fatalf("download header cookie = %q, want snapshot %q", got, "__puus=pre")
	}
	if !strings.Contains(d.Cookie, "__puus=rotated") {
		t.Fatalf("driver cookie should be updated with rotated value: %q", d.Cookie)
	}
	if link.URL != "https://cdn.example.com/f?sign=1" {
		t.Fatalf("link url = %q", link.URL)
	}
	if link.Header.Get("User-Agent") != "test-ua" || link.Header.Get("Referer") != "https://pan.quark.cn" {
		t.Fatalf("unexpected link headers: %v", link.Header)
	}
}

func TestRefreshJitterWithinBounds(t *testing.T) {
	const (
		min = -puusRefreshJitter
		max = puusRefreshJitter
	)
	for i := 0; i < 1000; i++ {
		d := refreshJitter()
		if d < min || d > max {
			t.Fatalf("refreshJitter() = %v, want within [%v, %v]", d, min, max)
		}
		if d%time.Second != 0 {
			t.Fatalf("refreshJitter() = %v, want precise to second", d)
		}
	}
}

var _ model.Obj = (*File)(nil)

// TestRefreshPuusFailurePreservesConcurrentUpdate 验证：刷新失败时，
// 如果并发请求已经带来了新的 __puus，恢复逻辑不能覆盖它
func TestRefreshPuusFailurePreservesConcurrentUpdate(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release // 阻塞，模拟慢响应，制造并发窗口
		writeJSON(w, 500, Resp{Status: 500, Code: 0, Message: "boom"})
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "a=1; __puus=old; b=2"

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.refreshPuus()
	}()

	<-started // 刷新请求已发出（此时请求头不带 __puus）
	// 模拟并发业务请求的响应合并了新的 __puus
	d.cookieMu.Lock()
	d.Cookie = cookie.SetStr(d.Cookie, "__puus", "concurrent-new")
	d.cookieMu.Unlock()
	close(release) // 放行，刷新请求返回 500

	if err := <-errCh; err == nil {
		t.Fatal("want error from refreshPuus")
	}
	// 并发写入的新值不能被失败恢复逻辑覆盖
	if !strings.Contains(d.Cookie, "__puus=concurrent-new") {
		t.Fatalf("concurrent __puus update was clobbered: %q", d.Cookie)
	}
}

// TestRefreshPuusDoesNotDisturbConcurrentDownload 验证：刷新请求在途时，
// d.Cookie 不会被 strip，并发下载仍使用完整 cookie 快照
func TestRefreshPuusDoesNotDisturbConcurrentDownload(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	rec := &recordHandler{handler: func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/clouddrive/config":
			close(started)
			<-release
			writeJSON(w, 200, Resp{Status: 200, Code: 0, Message: "ok"})
		case "/1/clouddrive/file/download":
			writeJSON(w, 200, map[string]interface{}{
				"status": 200, "code": 0, "message": "ok",
				"data": []map[string]string{{"download_url": "https://cdn.example.com/f?sign=1"}},
			})
		default:
			http.NotFound(w, r)
		}
	}}
	srv := httptest.NewServer(rec)
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "a=1; __puus=old; b=2"

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.refreshPuus()
	}()

	<-started // 刷新请求在途
	// 并发下载：应拿到完整（未 strip）的 cookie 快照
	link, err := d.getDownloadLink(&File{Fid: "f1"})
	if err != nil {
		t.Fatalf("getDownloadLink during refresh: %v", err)
	}
	if got := link.Header.Get("Cookie"); !strings.Contains(got, "__puus=old") {
		t.Fatalf("download header cookie during refresh = %q, want it to keep __puus", got)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("refreshPuus: %v", err)
	}
}
