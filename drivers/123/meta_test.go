package _123

import "testing"

func TestConfigRequiresProxy(t *testing.T) {
	if !config.MustProxy() {
		t.Fatal("123Pan downloads must be proxied because direct links are bound to the requester's IP")
	}
}
