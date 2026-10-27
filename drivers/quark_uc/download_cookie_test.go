package quark

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/cookie"
)

func TestRequestMergesAllSetCookiesWhenPuusPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "__puus=new; Path=/")
		w.Header().Add("Set-Cookie", "st=new-st; Path=/")
		w.Header().Add("Set-Cookie", "tfstk=new-tfstk; Path=/")
		writeJSON(w, http.StatusOK, Resp{Status: 200, Code: 0, Message: "ok"})
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "__puus=old; st=old-st; keep=yes"
	if _, err := d.request("/config", http.MethodGet, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"__puus=new", "st=new-st", "tfstk=new-tfstk", "keep=yes"} {
		if !strings.Contains(d.Cookie, want) {
			t.Fatalf("cookie missing %q: %q", want, d.Cookie)
		}
	}
	if strings.Contains(d.Cookie, "__puus=old") || strings.Contains(d.Cookie, "st=old-st") {
		t.Fatalf("old cookie value remained: %q", d.Cookie)
	}
}

func TestGetDownloadLinkUsesResponseMergedCookie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/1/clouddrive/file/download" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Cookie"); got != "__puus=before" {
			t.Errorf("download API Cookie = %q", got)
		}
		w.Header().Add("Set-Cookie", "__puus=after; Path=/")
		w.Header().Add("Set-Cookie", "st=st-after; Path=/")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []map[string]string{{"download_url": "https://cdn.example/file"}},
		})
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.Cookie = "__puus=before"
	link, err := d.getDownloadLink(&File{Fid: "f1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := link.Header.Get("Cookie"); cookie.GetStr(got, "__puus") != "after" || cookie.GetStr(got, "st") != "st-after" {
		t.Fatalf("link Cookie = %q, want response-merged cookie", got)
	}
	if got := d.Cookie; cookie.GetStr(got, "__puus") != "after" || cookie.GetStr(got, "st") != "st-after" {
		t.Fatalf("driver Cookie = %q", got)
	}
	if link.URL != "https://cdn.example/file" || link.Header.Get("User-Agent") != "test-ua" || link.Header.Get("Referer") != "https://pan.quark.cn" {
		t.Fatalf("unexpected download link: %+v", link)
	}
}

func TestConcurrentDownloadLinksKeepTheirOwnResponseCookie(t *testing.T) {
	const requests = 20
	oldClient := base.RestyClient
	base.RestyClient = base.NewRestyClient().SetCookieJar(nil)
	t.Cleanup(func() { base.RestyClient = oldClient })
	var mu sync.Mutex
	arrived := 0
	ready := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		arrived++
		id := arrived
		if arrived == requests {
			close(ready)
		}
		mu.Unlock()
		<-ready

		w.Header().Add("Set-Cookie", fmt.Sprintf("__puus=response-%d; Path=/", id))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": 200,
			"code":   0,
			"data": []map[string]string{{
				"download_url": fmt.Sprintf("https://cdn.example/file?id=%d", id),
			}},
		})
	}))
	defer srv.Close()

	d := newTestDriver(srv.URL)
	d.client = nil // 验证并发初始化独立、无 Cookie jar 的客户端。
	d.Cookie = "session=base; __puus=before"
	links := make(chan *model.Link, requests)
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := d.getDownloadLink(&File{Fid: "f1"})
			if err != nil {
				errs <- err
				return
			}
			links <- link
		}()
	}
	wg.Wait()
	close(links)
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}
	for link := range links {
		id := strings.TrimPrefix(link.URL, "https://cdn.example/file?id=")
		want := "response-" + id
		if got := cookie.GetStr(link.Header.Get("Cookie"), "__puus"); got != want {
			t.Fatalf("URL %q paired with cookie %q, want %q", link.URL, got, want)
		}
		if cookie.GetStr(link.Header.Get("Cookie"), "session") != "base" {
			t.Fatalf("request-local base cookie was lost: %q", link.Header.Get("Cookie"))
		}
	}
}
