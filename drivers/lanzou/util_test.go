package lanzou

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

func TestFindFileID(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
		ok   bool
	}{
		{name: "legacy single quoted path", html: `url: '/ajaxm.php?file=12345'`, want: "12345", ok: true},
		{name: "double quoted path", html: `url: "/ajaxm.php?file=23456"`, want: "23456", ok: true},
		{name: "absolute URL", html: `url: "https://example.lanzouv.com/ajaxm.php?file=34567"`, want: "34567", ok: true},
		{name: "unquoted expression", html: `fetch('/ajaxm.php?file=45678&p=1')`, want: "45678", ok: true},
		{name: "password file endpoint", html: `url : '/ajaxfile.php?file=215138709'`, want: "215138709", ok: true},
		{name: "f_id variable", html: `var f_id = '56789';`, want: "56789", ok: true},
		{name: "fid variable without quotes", html: `let fid=67890;`, want: "67890", ok: true},
		{name: "missing ID", html: `url: '/ajaxm.php?action=downprocess'`, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findFileID(tt.html)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("findFileID() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestGetAjaxURL(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{name: "legacy page with file ID", html: `url: '/ajaxm.php?file=12345'`, want: "https://pan.lanzoui.com/ajaxm.php?file=12345"},
		{name: "relative password file endpoint", html: `url : '/ajaxfile.php?file=215138709'`, want: "https://pan.lanzoui.com/ajaxfile.php?file=215138709"},
		{name: "absolute password file endpoint", html: `url : 'https://apifile.woozooo.com/ajaxfile.php?file=215138709'`, want: "https://apifile.woozooo.com/ajaxfile.php?file=215138709"},
		{name: "relative ajaxfiles endpoint", html: `url: '/ajaxfiles.php?file=12345'`, want: "https://pan.lanzoui.com/ajaxfiles.php?file=12345"},
		{name: "absolute ajaxfiles endpoint", html: `url: "https://download.example/ajaxfiles.php?file=12345"`, want: "https://download.example/ajaxfiles.php?file=12345"},
		{name: "new page without file ID", html: `data: {'action':'downprocess','sign':sign,'ves':1}`, want: "https://pan.lanzoui.com/ajaxm.php"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAjaxURL(tt.html, "https://pan.lanzoui.com"); got != tt.want {
				t.Fatalf("getAjaxURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetFilesByShareURLWithoutFileID(t *testing.T) {
	originalClient, originalNoRedirectClient := base.RestyClient, base.NoRedirectClient
	base.RestyClient = resty.New()
	base.NoRedirectClient = resty.New().SetRedirectPolicy(resty.RedirectPolicyFunc(
		func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	))
	t.Cleanup(func() {
		base.RestyClient = originalClient
		base.NoRedirectClient = originalNoRedirectClient
	})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fn":
			fmt.Fprint(w, `<script>var sign = 'test-sign'; data: {'action':'downprocess','sign':sign,'ves':1}</script>`)
		case "/ajaxm.php":
			if r.URL.RawQuery != "" {
				t.Errorf("unexpected ajaxm query: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"zt":1,"dom":%q,"url":"download","inf":0}`, server.URL)
		case "/file/download":
			w.Header().Set("Location", server.URL+"/direct")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	d := &LanZou{Addition: Addition{ShareUrl: server.URL, UserAgent: "test"}}
	sharePage := `<title>test.txt - 蓝奏云</title><iframe src="/fn"></iframe><span>大小 1 M</span>`
	file, err := d.getFilesByShareUrl("share-id", "", sharePage)
	if err != nil {
		t.Fatalf("getFilesByShareUrl() error = %v", err)
	}
	if file.Url != server.URL+"/direct" {
		t.Fatalf("file URL = %q, want %q", file.Url, server.URL+"/direct")
	}
}

func TestGetPasswordFileThroughAjaxfileEndpoint(t *testing.T) {
	originalClient, originalNoRedirectClient := base.RestyClient, base.NoRedirectClient
	base.RestyClient = resty.New()
	base.NoRedirectClient = resty.New().SetRedirectPolicy(resty.RedirectPolicyFunc(
		func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	))
	t.Cleanup(func() {
		base.RestyClient = originalClient
		base.NoRedirectClient = originalNoRedirectClient
	})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/share-id":
			http.SetCookie(w, &http.Cookie{Name: "share_session", Value: "ok", Path: "/"})
			fmt.Fprint(w, `<title>protected.apk - 蓝奏云</title><div id="passwddiv"></div><script>
function down_p(){
var sign = 'test-sign';
$.ajax({url:'/ajaxfile.php?file=215138709',data:{'action':'downprocess','sign':sign,'kd':1,'p':pwd}});
}</script><span>大小 1 M</span>`)
		case "/ajaxfile.php":
			if r.URL.Query().Get("file") != "215138709" {
				t.Errorf("unexpected file query: %q", r.URL.RawQuery)
			}
			cookie, err := r.Cookie("share_session")
			if err != nil || cookie.Value != "ok" {
				t.Errorf("share cookie missing: %v", err)
			}
			if got := r.Header.Get("Referer"); got != server.URL+"/share-id" {
				t.Errorf("Referer = %q", got)
			}
			if got := r.Header.Get("Origin"); got != server.URL {
				t.Errorf("Origin = %q", got)
			}
			if got := r.Header.Get("X-Requested-With"); got != "XMLHttpRequest" {
				t.Errorf("X-Requested-With = %q", got)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := r.Form.Get("p"); got != "1234" {
				t.Errorf("password = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"zt":1,"dom":%q,"url":"download","inf":"protected.apk"}`, server.URL)
		case "/file/download":
			w.Header().Set("Location", server.URL+"/direct")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	d := &LanZou{Addition: Addition{ShareUrl: server.URL, UserAgent: "test"}}
	file, err := d.GetFilesByShareUrl("share-id", "1234")
	if err != nil {
		t.Fatalf("GetFilesByShareUrl() error = %v", err)
	}
	if file.Url != server.URL+"/direct" {
		t.Fatalf("file URL = %q, want %q", file.Url, server.URL+"/direct")
	}
}
