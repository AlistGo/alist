package yunpan360

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

type authTransport struct{ target *url.URL }

func (a authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme, clone.URL.Host = a.target.Scheme, a.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func TestOpenAuthProtocolAndCache(t *testing.T) {
	for _, env := range []string{"prod", "test", "hgtest"} {
		t.Run(env, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				for k, v := range map[string]string{"method": "Oauth.getAccessTokenByApiKeyOrQT", "client_env": env, "client_src": "default", "grant_type": "authorization_code", "sub_channel": "open", "api_key": "test-key"} {
					if q.Get(k) != v {
						t.Errorf("%s=%q, want %q", k, q.Get(k), v)
					}
				}
				if r.Header.Get("api_key") != "test-key" {
					t.Error("missing API key header")
				}
				if q.Has("client_id") || q.Has("client_secret") {
					t.Error("legacy client credentials should not be sent")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"errno":0,"data":{"access_token":"access","token":"sign-token","qid":"user-id"}}`)
			}))
			defer srv.Close()
			target, _ := url.Parse(srv.URL)
			old := base.RestyClient
			base.RestyClient = resty.New().SetTransport(authTransport{target})
			defer func() { base.RestyClient = old }()
			d := &Yunpan360{Addition: Addition{APIKey: "test-key", EcsEnv: env, SubChannel: "open"}}
			for i := 0; i < 2; i++ {
				auth, err := d.getOpenAuth(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if auth.AccessToken != "access" || auth.Qid != "user-id" || auth.Token != "sign-token" {
					t.Fatalf("bad auth: %+v", auth)
				}
			}
			if calls != 1 {
				t.Fatalf("cache did not prevent duplicate request: %d", calls)
			}
			d.openAuthExpire = time.Now().Add(-time.Minute)
			if _, err := d.getOpenAuth(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatal("expired auth was not refreshed")
			}
		})
	}
}

func TestOpenAuthErrorDoesNotCache(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"array error", `{"errno":1001,"errmsg":"header error","data":[]}`, "header error"},
		{"object error", `{"errno":1002,"errmsg":"invalid api key","data":{}}`, "invalid api key"},
		{"missing message", `{"errno":1003,"data":[]}`, "yunpan request failed: errno=1003"},
		{"empty success", `{"errno":0,"data":{}}`, "yunpan auth returned an empty access token"},
		{"null success", `{"errno":0,"data":null}`, "yunpan auth returned an empty access token"},
		{"malformed", `not json`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, tt.body) }))
			defer srv.Close()
			target, _ := url.Parse(srv.URL)
			old := base.RestyClient
			base.RestyClient = resty.New().SetTransport(authTransport{target})
			defer func() { base.RestyClient = old }()
			d := &Yunpan360{Addition: Addition{APIKey: "test-key", EcsEnv: "prod", SubChannel: "open"}}
			for i := 0; i < 2; i++ {
				_, err := d.getOpenAuth(context.Background())
				if err == nil || (tt.want != "" && err.Error() != tt.want) {
					t.Fatalf("error=%v, want %q", err, tt.want)
				}
			}
			if calls != 2 || d.cachedOpenAuth != nil {
				t.Fatal("failed authentication was cached")
			}
		})
	}
}
