package v3_64_0

import (
	"context"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/server/common"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResetUserBasePathsRollsBackAndRetries(t *testing.T) {
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rollback.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { op.ClearUserCache(); op.ClearRoleCache(); sqlDB.Close() })
	if err := db.CreateRole(&model.Role{ID: 104, Name: "shared", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 1280}}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "fail"} {
		if err := db.CreateUser(&model.User{Username: name, BasePath: "/教学", Role: model.Roles{104}}); err != nil {
			t.Fatal(err)
		}
	}
	cached, err := op.GetUserByName("first")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("CREATE TRIGGER reject_update BEFORE UPDATE ON users WHEN OLD.username = 'fail' BEGIN SELECT RAISE(FAIL, 'injected failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := ResetUserBasePaths(); err == nil {
		t.Fatal("expected database failure")
	}
	for _, name := range []string{"first", "fail"} {
		u, err := db.GetUserByName(name)
		if err != nil || u.BasePath != "/教学" {
			t.Fatalf("partial migration for %s: %+v, %v", name, u, err)
		}
	}
	var roleCount int64
	database.Model(&model.Role{}).Count(&roleCount)
	if roleCount != 1 {
		t.Fatalf("rollback left roles: %d", roleCount)
	}
	if cached.BasePath != "/教学" {
		t.Fatal("rollback changed cached user")
	}
	if err := database.Exec("DROP TRIGGER reject_update").Error; err != nil {
		t.Fatal(err)
	}
	if err := ResetUserBasePaths(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "fail"} {
		u, err := op.GetUserByName(name)
		if err != nil || u.BasePath != "/" {
			t.Fatalf("retry did not reset %s: %+v, %v", name, u, err)
		}
	}
	database.Model(&model.Role{}).Count(&roleCount)
	if err := ResetUserBasePaths(); err != nil {
		t.Fatal(err)
	}
	var after int64
	database.Model(&model.Role{}).Count(&after)
	if after != roleCount {
		t.Fatalf("retry duplicated roles: %d -> %d", roleCount, after)
	}
}

func TestMigrationPreservesRoleBoundariesAndIdentity(t *testing.T) {
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "scopes.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, _ := database.DB()
	op.ClearRoleCache()
	op.ClearUserCache()
	t.Cleanup(func() { op.ClearUserCache(); op.ClearRoleCache(); sqlDB.Close() })
	roles := []model.Role{
		{ID: 1, Name: "guest", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 0}}},
		{ID: 2, Name: "admin", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 65535}}},
		{ID: 104, Name: "reader", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 1280}}},
		{ID: 105, Name: "writer", PermissionScopes: []model.PermissionEntry{{Path: "/教学/作业", Permission: 8}, {Path: "/财务", Permission: 128}}},
		{ID: 106, Name: "migrated-base-path-user-1", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 65535}}},
	}
	for i := range roles {
		if err := db.CreateRole(&roles[i]); err != nil {
			t.Fatal(err)
		}
	}
	users := []model.User{
		{Username: "restricted", BasePath: "/教学/", Role: model.Roles{104, 105}, PwdHash: "hash", Salt: "salt", PwdTS: 123},
		{Username: "shared-root", BasePath: "/", Role: model.Roles{104}},
		{Username: "guest", BasePath: "/公开", Role: model.Roles{1}},
		{Username: "guest-sharer", BasePath: "/", Role: model.Roles{1}},
		{Username: "admin", BasePath: "/old-admin", Role: model.Roles{2}},
		{Username: "disjoint", BasePath: "/别处", Role: model.Roles{105}, Disabled: true},
		{Username: "empty", BasePath: "", Role: model.Roles{104}},
	}
	for i := range users {
		if err := db.CreateUser(&users[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Create(&model.SettingItem{Key: conf.DefaultRole, Value: "guest"}).Error; err != nil {
		t.Fatal(err)
	}
	if op.GetDefaultRoleID() != model.GUEST {
		t.Fatal("invalid default fixture")
	}
	// Prime both user and role caches before the migration.
	if _, err := op.GetRole(1); err != nil {
		t.Fatal(err)
	}
	if _, err := op.GetUserByName("restricted"); err != nil {
		t.Fatal(err)
	}
	if err := ResetUserBasePaths(); err != nil {
		t.Fatal(err)
	}
	get := func(name string) *model.User {
		t.Helper()
		u, err := op.GetUserByName(name)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	u := get("restricted")
	if u.BasePath != "/" || len(u.Role) != 1 || u.Role.Contains(104) || u.Role.Contains(105) {
		t.Fatalf("bindings: %+v", u)
	}
	if !common.CanReadPathByRole(u, "/教学/课件") || common.CanReadPathByRole(u, "/教学外") || common.CanReadPathByRole(u, "/财务") {
		t.Fatal("scope expanded or lost")
	}
	for p, want := range map[string]int32{"/": 0, "/教学/课件": 1280, "/教学/作业/一": 1288, "/财务": 0} {
		if got := common.MergeRolePermissions(u, p); got != want {
			t.Errorf("%s: got %d want %d", p, got, want)
		}
	}
	ctx := context.WithValue(context.Background(), "user", u)
	for _, dst := range []string{"/", "/财务", "/教学外"} {
		if err := fs.Move(ctx, "/教学/课件", dst); !errors.Is(err, errs.PermissionDenied) {
			t.Fatalf("move destination %s: %v", dst, err)
		}
		if _, err := fs.Copy(ctx, "/教学/课件", dst); !errors.Is(err, errs.PermissionDenied) {
			t.Fatalf("copy destination %s: %v", dst, err)
		}
	}
	if !common.IsPathInRoleScope(u, "/教学/作业") {
		t.Fatal("allowed destination lost")
	}
	if !common.HasChildPermission(u, "/", common.PermFTPAccess) {
		t.Fatal("migration prevents scoped FTP/SFTP login")
	}
	if u.PwdHash != "hash" || u.Salt != "salt" || u.PwdTS != 123 {
		t.Fatal("credentials changed")
	}
	r, err := db.GetRole(uint(u.Role[0]))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name == roles[4].Name || r.Default {
		t.Fatal("role collision or default role changed")
	}
	if !reflect.DeepEqual(get("shared-root").Role, users[1].Role) {
		t.Fatal("unrelated user changed")
	}
	defaultID := op.GetDefaultRoleID()
	if defaultID == model.GUEST {
		t.Fatal("default registration still uses empty identity role")
	}
	template, err := op.GetRole(uint(defaultID))
	if err != nil || !reflect.DeepEqual(template.PermissionScopes, roles[0].PermissionScopes) {
		t.Fatalf("default grants changed: %+v %v", template, err)
	}
	guest := get("guest")
	if !guest.IsGuest() || common.CanReadPathByRole(guest, "/private") || !common.CanReadPathByRole(guest, "/公开/文件") {
		t.Fatal("guest identity/scope changed")
	}
	if g, err := op.GetGuest(); err != nil || g.ID != guest.ID {
		t.Fatalf("guest discovery failed: %+v %v", g, err)
	}
	if !reflect.DeepEqual(get("guest-sharer").Role, model.Roles{model.GUEST, defaultID}) {
		t.Fatal("root guest must reuse shared template rather than a new per-user role")
	}
	for _, user := range []model.User{users[1], users[3], users[6]} {
		var count int64
		database.Model(&model.Role{}).Where("name LIKE ?", fmt.Sprintf("migrated-base-path-user-%d%%", user.ID)).Count(&count)
		if count != 0 {
			t.Fatalf("root user %s received a new role", user.Username)
		}
	}
	if !get("guest-sharer").IsGuest() || !common.CanReadPathByRole(get("guest-sharer"), "/private") {
		t.Fatal("shared guest role lost original access")
	}
	if !get("admin").IsAdmin() || get("admin").BasePath != "/" {
		t.Fatal("admin identity lost")
	}
	if common.CanReadPathByRole(get("disjoint"), "/别处/file") || !get("disjoint").Disabled {
		t.Fatal("disjoint role gained access")
	}
	if !reflect.DeepEqual(get("empty").Role, users[6].Role) {
		t.Fatal("empty base should normalize without new role")
	}
	for _, old := range roles[1:] {
		got, err := db.GetRole(old.ID)
		if err != nil || !reflect.DeepEqual(got.PermissionScopes, old.PermissionScopes) {
			t.Fatalf("shared role changed: %+v %v", got, err)
		}
	}
	var before, after int64
	database.Model(&model.Role{}).Count(&before)
	if err := ResetUserBasePaths(); err != nil {
		t.Fatal(err)
	}
	database.Model(&model.Role{}).Count(&after)
	if before != after {
		t.Fatal("second migration creates roles")
	}
}
