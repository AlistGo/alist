package utils

import "testing"

func TestEncodePath(t *testing.T) {
	t.Log(EncodePath("http://localhost:5244/d/123#.png"))
}

func TestFixAndCleanPath(t *testing.T) {
	datas := map[string]string{
		"":                          "/",
		".././":                     "/",
		"../../.../":                "/...",
		"x//\\y/":                   "/x/y",
		".././.x/.y/.//..x../..y..": "/.x/.y/..x../..y..",
	}
	for key, value := range datas {
		if FixAndCleanPath(key) != value {
			t.Logf("raw %s fix fail", key)
		}
	}
}

func TestValidateNameComponent(t *testing.T) {
	validNames := []string{
		"file.txt",
		"abc",
		"file_name-1",
		"a..b",
	}
	for _, name := range validNames {
		if err := ValidateNameComponent(name); err != nil {
			t.Fatalf("expected valid name %q, got error: %v", name, err)
		}
	}

	invalidNames := []string{
		"",
		".",
		"..",
		"a/b",
		`a\b`,
		"../a",
		`..\a`,
		string([]byte{'a', 0, 'b'}),
	}
	for _, name := range invalidNames {
		if err := ValidateNameComponent(name); err == nil {
			t.Fatalf("expected invalid name %q to be rejected", name)
		}
	}
}

func TestJoinUnderBase(t *testing.T) {
	base := "/lanzou-y/shared/test1"
	out, err := JoinUnderBase(base, "file.txt")
	if err != nil {
		t.Fatalf("expected join success, got error: %v", err)
	}
	if out != "/lanzou-y/shared/test1/file.txt" {
		t.Fatalf("unexpected join result: %s", out)
	}

	if _, err := JoinUnderBase(base, "../admin/screts.txt"); err == nil {
		t.Fatalf("expected traversal to be rejected")
	}
	if _, err := JoinUnderBase(base, "sub/child"); err == nil {
		t.Fatalf("expected nested path to be rejected")
	}
}

func TestJoinBasePath(t *testing.T) {
	// A user's base path confines every request path, not just "/".
	datas := []struct {
		basePath string
		reqPath  string
		want     string
	}{
		{"/public", "/", "/public"},
		{"/public", "/sub", "/public/sub"},
		{"/public", "sub", "/public/sub"},
		{"/public", "/sub/file.txt", "/public/sub/file.txt"},
		{"/public", "/other_storage", "/public/other_storage"},
		{"/", "/sub", "/sub"},
		{"", "/sub", "/sub"},
	}
	for _, d := range datas {
		got, err := JoinBasePath(d.basePath, d.reqPath)
		if err != nil {
			t.Fatalf("JoinBasePath(%q, %q) error: %v", d.basePath, d.reqPath, err)
		}
		if got != d.want {
			t.Errorf("JoinBasePath(%q, %q) = %q, want %q", d.basePath, d.reqPath, got, d.want)
		}
	}

	// relative segments must still be rejected
	for _, reqPath := range []string{"..", "../x", "/x/..", "/x/../y"} {
		if _, err := JoinBasePath("/public", reqPath); err == nil {
			t.Errorf("JoinBasePath(%q, %q) expected error, got nil", "/public", reqPath)
		}
	}
}
