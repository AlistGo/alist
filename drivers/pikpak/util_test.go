package pikpak

import "testing"

func TestRewriteDownloadURL(t *testing.T) {
	cases := []struct {
		name           string
		downloadDomain string // selected download_domain
		customDomain   string // custom_download_domain (overrides selection)
		in             string
		want           string
	}{
		{"original is no-op", "original", "", "https://dl-a.mypikpak.com/x?sig=1", "https://dl-a.mypikpak.com/x?sig=1"},
		{"empty is no-op", "", "", "https://dl-a.mypikpak.com/x?sig=1", "https://dl-a.mypikpak.com/x?sig=1"},
		{"option key swaps com to net keeps subdomain and query", "mypikpak_net", "", "https://dl-a10b.mypikpak.com/download/f?X-Amz-Signature=abc", "https://dl-a10b.mypikpak.net/download/f?X-Amz-Signature=abc"},
		{"option key swaps net to com", "mypikpak_com", "", "https://vip-lixian-07.mypikpak.net/y", "https://vip-lixian-07.mypikpak.com/y"},
		{"option key swaps to pikpak.me", "pikpak_me", "", "https://dl-a.mypikpak.com/z", "https://dl-a.pikpak.me/z"},
		{"raw domain value also works", "mypikpak.net", "", "https://dl-a.mypikpak.com/z", "https://dl-a.mypikpak.net/z"},
		{"custom overrides selection", "mypikpak_net", "dl.example.org", "https://dl-a.mypikpak.com/z", "https://dl-a.dl.example.org/z"},
		{"custom overrides original", "original", "pikpak.me", "https://dl-a.mypikpak.com/z", "https://dl-a.pikpak.me/z"},
		{"bare host without subdomain", "mypikpak_net", "", "https://mypikpak.com/z", "https://mypikpak.net/z"},
		{"unknown host is left untouched", "mypikpak_net", "", "https://cdn.example.com/z", "https://cdn.example.com/z"},
		{"empty url is no-op", "mypikpak_net", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := &PikPak{Addition: Addition{DownloadDomain: c.downloadDomain, CustomDownloadDomain: c.customDomain}}
			if got := d.rewriteDownloadURL(c.in); got != c.want {
				t.Fatalf("rewriteDownloadURL(%q) sel=%q custom=%q = %q, want %q", c.in, c.downloadDomain, c.customDomain, got, c.want)
			}
		})
	}
}

func TestApiAndUserURL(t *testing.T) {
	cases := []struct {
		apiDomain    string // selected api_domain
		customDomain string // custom_api_domain (overrides selection)
		wantAPI      string
		wantUser     string
	}{
		{"", "", "https://api-drive.mypikpak.net/drive/v1/files", "https://user.mypikpak.net/v1/auth/token"},
		{"mypikpak_com", "", "https://api-drive.mypikpak.com/drive/v1/files", "https://user.mypikpak.com/v1/auth/token"},
		{"pikpak_me", "", "https://api-drive.pikpak.me/drive/v1/files", "https://user.pikpak.me/v1/auth/token"},
		{"mypikpak.com", "", "https://api-drive.mypikpak.com/drive/v1/files", "https://user.mypikpak.com/v1/auth/token"},
		{"mypikpak_net", "api.example.org", "https://api-drive.api.example.org/drive/v1/files", "https://user.api.example.org/v1/auth/token"},
	}
	for _, c := range cases {
		d := &PikPak{Addition: Addition{ApiDomain: c.apiDomain, CustomApiDomain: c.customDomain}}
		if got := d.apiURL("/drive/v1/files"); got != c.wantAPI {
			t.Fatalf("apiURL sel=%q custom=%q = %q, want %q", c.apiDomain, c.customDomain, got, c.wantAPI)
		}
		if got := d.userURL("/v1/auth/token"); got != c.wantUser {
			t.Fatalf("userURL sel=%q custom=%q = %q, want %q", c.apiDomain, c.customDomain, got, c.wantUser)
		}
	}
}
