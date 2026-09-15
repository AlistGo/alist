package bootstrap

import (
	"encoding/json"
	"errors"
	"github.com/alist-org/alist/v3/cmd/flags"
	"github.com/alist-org/alist/v3/internal/bootstrap/patch"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestUpgradeResetsUserBasePaths(t *testing.T) {
	oldConf, oldVersion, oldLast := conf.Conf, conf.Version, LastLaunchedVersion
	oldDB := db.GetDb()
	oldDataDir := flags.DataDir
	flags.DataDir = t.TempDir()
	t.Cleanup(func() {
		conf.Conf, conf.Version, LastLaunchedVersion = oldConf, oldVersion, oldLast
		flags.DataDir = oldDataDir
		op.ClearUserCache()
		if oldDB != nil {
			db.Init(oldDB)
		}
	})
	conf.Conf = conf.DefaultConfig()
	conf.Conf.Database.TablePrefix = "migration_"
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: conf.Conf.Database.TablePrefix},
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	for _, r := range []model.Role{{ID: 1, Name: "guest", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 0}}}, {ID: 2, Name: "admin", PermissionScopes: []model.PermissionEntry{{Path: "/", Permission: 65535}}}} {
		if err := db.CreateRole(&r); err != nil {
			t.Fatal(err)
		}
	}
	role := model.Role{ID: 104, Name: "teaching", PermissionScopes: []model.PermissionEntry{{Path: "/教学", Permission: 256}}}
	if err := db.CreateRole(&role); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, previous, current string
		migrate                 bool
	}{
		{"older release", "v3.63.0", "v3.64.1", true},
		{"affected release", "v3.64.0", "v3.64.1", true},
		{"skip releases", "v3.64.0", "v3.65.0", true},
		{"same release restart", "v3.64.1", "v3.64.1", false},
		{"already upgraded", "v3.64.1", "v3.65.0", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users := []model.User{
				{Username: t.Name() + "-ordinary", BasePath: "/教学", Role: model.Roles{104}, Permission: 256, PwdHash: "hash", Salt: "salt", PwdTS: 123, OtpSecret: "otp", Authn: "[]", SsoID: "sso"},
				{Username: t.Name() + "-guest", BasePath: "/guest", Role: model.Roles{model.GUEST}},
				{Username: t.Name() + "-admin", BasePath: "/admin", Role: model.Roles{model.ADMIN}},
				{Username: t.Name() + "-disabled", BasePath: "/custom", Role: model.Roles{104}, Disabled: true},
				{Username: t.Name() + "-empty", BasePath: "", Role: model.Roles{104}},
				{Username: t.Name() + "-root", BasePath: "/", Role: model.Roles{104}},
			}
			for i := range users {
				if err := db.CreateUser(&users[i]); err != nil {
					t.Fatal(err)
				}
				id := users[i].ID
				t.Cleanup(func() { db.DeleteUserById(id) })
			}
			if _, err := op.GetUserByName(users[0].Username); err != nil {
				t.Fatal(err)
			}
			conf.Version, LastLaunchedVersion = tt.current, tt.previous
			writeTestVersion(t, tt.previous)
			if err := InitUpgradePatch(); err != nil {
				t.Fatal(err)
			}
			// Re-running the migration must be harmless as well as restart-safe.
			if err := InitUpgradePatch(); err != nil {
				t.Fatal(err)
			}
			for _, want := range users {
				if tt.migrate {
					want.BasePath = "/"
				}
				got, err := db.GetUserById(want.ID)
				if err != nil {
					t.Fatal(err)
				}
				if tt.migrate && !want.IsAdmin() && want.Username != users[4].Username && want.Username != users[5].Username {
					if len(got.Role) == 0 {
						t.Fatal("missing migrated role")
					}
					want.Role = got.Role
				}
				if !reflect.DeepEqual(*got, want) {
					t.Errorf("user %s changed unexpectedly: got %+v, want %+v", want.Username, got, want)
				}
			}
			cached, err := op.GetUserByName(users[0].Username)
			if err != nil {
				t.Fatal(err)
			}
			wantBase := users[0].BasePath
			if tt.migrate {
				wantBase = "/"
			}
			if cached.BasePath != wantBase {
				t.Errorf("cached base path = %q, want %q", cached.BasePath, wantBase)
			}
		})
	}
	gotRole, err := db.GetRole(role.ID)
	if err != nil || !reflect.DeepEqual(gotRole.PermissionScopes, role.PermissionScopes) {
		t.Fatalf("role permissions changed: %+v, %v", gotRole, err)
	}
}

func writeTestVersion(t *testing.T, version string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"last_launched_version": version, "jwt_secret": "on-disk-value"})
	if err := os.WriteFile(filepath.Join(flags.DataDir, "config.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeFailureKeepsVersionForRetry(t *testing.T) {
	oldConf, oldVersion, oldLast, oldDir := conf.Conf, conf.Version, LastLaunchedVersion, flags.DataDir
	oldPatches := patch.UpgradePatches
	t.Cleanup(func() {
		conf.Conf, conf.Version, LastLaunchedVersion, flags.DataDir = oldConf, oldVersion, oldLast, oldDir
		patch.UpgradePatches = oldPatches
	})
	flags.DataDir = t.TempDir()
	conf.Conf = conf.DefaultConfig()
	conf.Conf.JwtSecret = "runtime-env-value"
	conf.Version, LastLaunchedVersion = "v3.64.1", "v3.64.0"
	writeTestVersion(t, LastLaunchedVersion)
	fail := true
	calls := 0
	patch.UpgradePatches = []patch.VersionPatches{{Version: "v3.64.0", Patches: []func() error{func() error {
		calls++
		if fail {
			return errors.New("injected failure")
		}
		return nil
	}}}}
	if err := InitUpgradePatch(); err == nil {
		t.Fatal("migration failure ignored")
	}
	readConfig := func() map[string]string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(flags.DataDir, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]string
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}
	if got := readConfig()["last_launched_version"]; got != "v3.64.0" {
		t.Fatalf("failure advanced version to %s", got)
	}
	fail = false
	if err := InitUpgradePatch(); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(); got["last_launched_version"] != "v3.64.1" || got["jwt_secret"] != "on-disk-value" {
		t.Fatalf("unexpected persisted config: %+v", got)
	}
	if err := InitUpgradePatch(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("migration calls = %d, want failed attempt + successful retry", calls)
	}
}

func TestUpgradePanicIsFailure(t *testing.T) {
	if err := safeCall("v3.64.0", 0, func() error { panic("injected panic") }); err == nil {
		t.Fatal("panicking patch reported success")
	}
}
