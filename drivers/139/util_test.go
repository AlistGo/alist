package _139

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

func TestSanitizeLoginCookiesReplacesJSessionIDAndOrdersAllowlist(t *testing.T) {
	got := sanitizeLoginCookies("unknown=x; RMKEY=rm; JSESSIONID=old; Os_SSo_Sid=sid; behaviorid=b", "fresh")
	want := "behaviorid=b; Os_SSo_Sid=sid; JSESSIONID=fresh"
	if got != want {
		t.Fatalf("sanitizeLoginCookies() = %q, want %q", got, want)
	}
}

func TestSanitizeLoginCookiesDropsStaleJSessionIDWhenFreshOneMissing(t *testing.T) {
	got := sanitizeLoginCookies("JSESSIONID=old; Os_SSo_Sid=sid", "")
	want := "Os_SSo_Sid=sid"
	if got != want {
		t.Fatalf("sanitizeLoginCookies() = %q, want %q", got, want)
	}
}

func TestMergeMailCookiesIsDeterministicAndKeepsExtrasSorted(t *testing.T) {
	got := mergeMailCookies("z=zv; behaviorid=b; Os_SSo_Sid=old", []*http.Cookie{
		{Name: "RMKEY", Value: "rm"},
		{Name: "Os_SSo_Sid", Value: "sid"},
		{Name: "a", Value: "av"},
	})
	want := "behaviorid=b; Os_SSo_Sid=sid; RMKEY=rm; a=av; z=zv"
	if got != want {
		t.Fatalf("mergeMailCookies() = %q, want %q", got, want)
	}
}

func TestExtractFastLoginCookies(t *testing.T) {
	sid, rmkey := extractFastLoginCookies("RMKEY=rm; Os_SSo_Sid=sid")
	if sid != "sid" || rmkey != "rm" {
		t.Fatalf("extractFastLoginCookies() = %q, %q; want sid, rm", sid, rmkey)
	}
}

func TestCredentialState(t *testing.T) {
	tests := []struct {
		name string
		d    Yun139
		want credentialState
		err  bool
	}{
		{
			name: "authorization",
			d:    Yun139{Addition: Addition{Authorization: " auth "}},
			want: credentialStateAuthorization,
		},
		{
			name: "full login",
			d: Yun139{Addition: Addition{
				MailCookies: "RMKEY=rm; Os_SSo_Sid=sid",
				Username:    "user",
				Password:    "password",
			}},
			want: credentialStateFullLogin,
		},
		{
			name: "cookies only",
			d:    Yun139{Addition: Addition{MailCookies: "RMKEY=rm; Os_SSo_Sid=sid"}},
			want: credentialStateCookiesOnly,
		},
		{
			name: "partial password login",
			d:    Yun139{Addition: Addition{Username: "user"}},
			err:  true,
		},
		{
			name: "missing credentials",
			d:    Yun139{},
			err:  true,
		},
		{
			name: "invalid cookie",
			d:    Yun139{Addition: Addition{MailCookies: "invalid-cookie"}},
			err:  true,
		},
		{
			name: "authorization with basic prefix",
			d:    Yun139{Addition: Addition{Authorization: "Basic abc"}},
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.d.credentialState()
			if tt.err {
				if err == nil {
					t.Fatal("credentialState() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("credentialState() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("credentialState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRedirectStatus(t *testing.T) {
	for _, status := range []int{300, 301, 302, 307, 399} {
		if !isRedirectStatus(status) {
			t.Fatalf("isRedirectStatus(%d) = false, want true", status)
		}
	}
	for _, status := range []int{200, 299, 400, 500} {
		if isRedirectStatus(status) {
			t.Fatalf("isRedirectStatus(%d) = true, want false", status)
		}
	}
}

func TestIntegrationLoginObtainsAuthorization(t *testing.T) {
	if os.Getenv("ALIST_139_INTEGRATION") != "1" {
		t.Skip("set ALIST_139_INTEGRATION=1 to run live 139Yun login checks")
	}
	base.RestyClient = resty.New().
		SetHeader("user-agent", base.UserAgent).
		SetRetryCount(3).
		SetRetryResetReaders(true).
		SetTimeout(30 * time.Second)

	username := os.Getenv("ALIST_139_USERNAME")
	password := os.Getenv("ALIST_139_PASSWORD")
	mailCookies := os.Getenv("ALIST_139_MAIL_COOKIES")
	authorization := strings.TrimSpace(os.Getenv("ALIST_139_AUTHORIZATION"))

	if authorization != "" {
		t.Run("authorization", func(t *testing.T) {
			d := Yun139{Addition: Addition{Authorization: authorization}}
			state, err := d.credentialState()
			if err != nil {
				t.Fatalf("credentialState() unexpected error: %v", err)
			}
			if state != credentialStateAuthorization {
				t.Fatalf("credentialState() = %v, want authorization", state)
			}
			if d.Authorization == "" || strings.HasPrefix(strings.ToLower(d.Authorization), "basic ") {
				t.Fatal("authorization should be present without Basic prefix")
			}
		})
	}

	if mailCookies == "" {
		t.Fatal("ALIST_139_MAIL_COOKIES is required")
	}

	runFastLogin := func(t *testing.T, mailCookies string) {
		t.Helper()
		d := Yun139{Addition: Addition{MailCookies: mailCookies}}
		state, err := d.credentialState()
		if err != nil {
			t.Fatalf("credentialState() unexpected error: %v", err)
		}
		if state != credentialStateCookiesOnly {
			t.Fatalf("credentialState() = %v, want cookies only", state)
		}
		sid, rmkey := extractFastLoginCookies(d.MailCookies)
		if sid == "" || rmkey == "" {
			t.Fatal("mail cookies are missing Os_SSo_Sid or RMKEY")
		}
		token, err := d.step2_get_single_token(sid)
		if err != nil {
			t.Fatalf("step2_get_single_token() error: %v", err)
		}
		auth, err := d.step3_third_party_login(token)
		if err != nil {
			t.Fatalf("step3_third_party_login() error: %v", err)
		}
		d.Authorization = auth
		if d.Authorization == "" {
			t.Fatal("authorization is empty after fast login")
		}
	}

	if username == "" || password == "" {
		t.Fatal("ALIST_139_USERNAME and ALIST_139_PASSWORD are required for password fallback")
	}

	var refreshedMailCookies string
	var generatedAuthorization string
	t.Run("password login fallback", func(t *testing.T) {
		d := Yun139{Addition: Addition{
			MailCookies: mailCookies,
			Username:    username,
			Password:    password,
		}}
		state, err := d.credentialState()
		if err != nil {
			t.Fatalf("credentialState() unexpected error: %v", err)
		}
		if state != credentialStateFullLogin {
			t.Fatalf("credentialState() = %v, want full login", state)
		}
		passId, err := d.step1_password_login()
		if err != nil {
			t.Fatalf("step1_password_login() error: %v", err)
		}
		token, err := d.step2_get_single_token(passId)
		if err != nil {
			t.Fatalf("step2_get_single_token() error: %v", err)
		}
		auth, err := d.step3_third_party_login(token)
		if err != nil {
			t.Fatalf("step3_third_party_login() error: %v", err)
		}
		d.Authorization = auth
		if auth == "" || d.Authorization == "" {
			t.Fatal("authorization is empty after password login")
		}
		generatedAuthorization = auth
		refreshedMailCookies = d.MailCookies
	})

	t.Run("authorization generated by password login", func(t *testing.T) {
		if generatedAuthorization == "" {
			t.Fatal("password login did not generate authorization")
		}
		d := Yun139{Addition: Addition{Authorization: generatedAuthorization}}
		state, err := d.credentialState()
		if err != nil {
			t.Fatalf("credentialState() unexpected error: %v", err)
		}
		if state != credentialStateAuthorization {
			t.Fatalf("credentialState() = %v, want authorization", state)
		}
		if strings.HasPrefix(strings.ToLower(d.Authorization), "basic ") {
			t.Fatal("authorization should not include Basic prefix")
		}
	})

	t.Run("mail cookies fast login from input", func(t *testing.T) {
		sid, rmkey := extractFastLoginCookies(mailCookies)
		if sid == "" || rmkey == "" {
			t.Skip("input mail cookies are missing Os_SSo_Sid or RMKEY")
		}
		runFastLogin(t, mailCookies)
	})

	t.Run("mail cookies fast login after password login", func(t *testing.T) {
		if refreshedMailCookies == "" {
			t.Fatal("password login did not refresh mail cookies")
		}
		runFastLogin(t, refreshedMailCookies)
	})
}
