package op_test

import (
	"testing"

	_ "github.com/alist-org/alist/v3/drivers"
	"github.com/alist-org/alist/v3/internal/op"
)

func TestDriverItemsMap(t *testing.T) {
	itemsMap := op.GetDriverInfoMap()
	if len(itemsMap) != 0 {
		t.Logf("driverInfoMap: %v", itemsMap)
	} else {
		t.Errorf("expected driverInfoMap not empty, but got empty")
	}
}

func TestDriverItemsShowWhen(t *testing.T) {
	info, ok := op.GetDriverInfoMap()["123 Open"]
	if !ok {
		t.Fatal("123 Open driver not registered")
	}
	want := map[string]string{
		"auth_mode":     "",
		"access_token":  "auth_mode=token",
		"refresh_token": "auth_mode=token",
		"client_id":     "auth_mode=client_credentials",
		"client_secret": "auth_mode=client_credentials",
	}
	got := map[string]string{}
	for _, item := range info.Additional {
		if item.Name == "auth_mode" && item.Default != "token" {
			t.Errorf("auth_mode default = %q, want token", item.Default)
		}
		if _, care := want[item.Name]; care {
			got[item.Name] = item.ShowWhen
		}
	}
	for name, cond := range want {
		if got[name] != cond {
			t.Errorf("%s show_when = %q, want %q", name, got[name], cond)
		}
	}
}
