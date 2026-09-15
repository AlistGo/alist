package op_test

import (
	"testing"

	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
)

func TestCreateUserUsesRootBasePath(t *testing.T) {
	role := model.Role{
		ID:   104,
		Name: t.Name(),
		PermissionScopes: []model.PermissionEntry{
			{Path: "/教学", Permission: 0},
			{Path: "/其他", Permission: 0},
		},
	}
	if err := db.CreateRole(&role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DeleteRole(role.ID) })

	for _, tt := range []struct {
		name string
		base string
		want string
	}{
		{name: "root", base: "/", want: "/"},
		{name: "empty", base: "", want: "/"},
		{name: "explicit", base: "/custom/", want: "/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			user := model.User{
				Username: t.Name(),
				BasePath: tt.base,
				Role:     model.Roles{int(role.ID)},
			}
			if err := op.CreateUser(&user); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.DeleteUserById(user.ID) })
			stored, err := db.GetUserById(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.BasePath != tt.want {
				t.Fatalf("persisted base path = %q, want %q", stored.BasePath, tt.want)
			}
			if tt.want == "/" {
				got, err := stored.JoinPath("/教学")
				if err != nil || got != "/教学" {
					t.Fatalf("role path resolved to %q, %v; want /教学", got, err)
				}
			}
		})
	}
}

func TestUpdateUserCannotRestoreBasePath(t *testing.T) {
	user := model.User{Username: t.Name(), BasePath: "/", Role: model.Roles{104}, Disabled: true, PwdHash: "unchanged"}
	if err := op.CreateUser(&user); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DeleteUserById(user.ID) })
	if _, err := op.GetUserByName(user.Username); err != nil {
		t.Fatal(err)
	}
	user.BasePath = "/教学"
	if err := op.UpdateUser(&user); err != nil {
		t.Fatal(err)
	}
	got, err := op.GetUserByName(user.Username)
	if err != nil {
		t.Fatal(err)
	}
	if got.BasePath != "/" || !got.Disabled || got.PwdHash != "unchanged" || len(got.Role) != 1 || got.Role[0] != 104 {
		t.Fatalf("unexpected user after update: %+v", got)
	}
}
