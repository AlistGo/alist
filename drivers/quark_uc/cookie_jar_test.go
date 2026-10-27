package quark

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

func TestDownloadCookiesDoNotShareGlobalJar(t *testing.T) {
	const configuredCookie = "__puus=configured; __uid=own-account"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/seed" {
			w.Header().Add("Set-Cookie", "__puus=cached; Path=/")
			w.Header().Add("Set-Cookie", "__uid=other-account; Path=/")
			writeJSON(w, http.StatusOK, Resp{Status: 200})
			return
		}
		if got := r.Header.Get("Cookie"); got != configuredCookie {
			t.Errorf("API Cookie = %q, want only configured cookie %q", got, configuredCookie)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []map[string]string{{"download_url": "https://cdn.example/file"}},
		})
	}))
	defer srv.Close()

	oldClient := base.RestyClient
	base.RestyClient = resty.New()
	t.Cleanup(func() { base.RestyClient = oldClient })
	if _, err := base.RestyClient.R().Get(srv.URL + "/seed"); err != nil {
		t.Fatal(err)
	}

	d := newTestDriver(srv.URL)
	d.client = nil // 全局 Cookie jar 已有缓存，验证实际客户端路径不受其影响。
	d.Cookie = configuredCookie
	for i := 0; i < 3; i++ {
		link, err := d.getDownloadLink(&File{Fid: "f1"})
		if err != nil {
			t.Fatal(err)
		}
		if got := link.Header.Get("Cookie"); got != configuredCookie {
			t.Fatalf("download Cookie = %q, want %q", got, configuredCookie)
		}
	}
}

func TestRequestDoesNotCacheIgnoredResponseCookies(t *testing.T) {
	const configuredCookie = "__puus=configured; __uid=own-account"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != configuredCookie {
			t.Errorf("API Cookie = %q, want only configured cookie %q", got, configuredCookie)
		}
		// 不含 __puus 的响应 Cookie 不应影响后续请求。
		w.Header().Add("Set-Cookie", "__uid=unexpected-account; Path=/")
		writeJSON(w, http.StatusOK, Resp{Status: 200})
	}))
	defer srv.Close()

	oldClient := base.RestyClient
	base.RestyClient = resty.New()
	t.Cleanup(func() { base.RestyClient = oldClient })
	d := newTestDriver(srv.URL)
	d.client = nil
	d.Cookie = configuredCookie
	for i := 0; i < 3; i++ {
		if _, err := d.request("/config", http.MethodGet, nil, nil); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(d.Cookie, "unexpected-account") {
			t.Fatal("ignored response cookie was saved")
		}
	}
}
