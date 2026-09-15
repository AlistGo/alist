package handles

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserEntryPointsUseRootBasePath(t *testing.T) {
	oldConf := conf.Conf
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { op.ClearUserCache(); op.SettingCacheUpdate(); sqlDB.Close(); conf.Conf = oldConf })
	role := model.Role{ID: 104, Name: "scoped", PermissionScopes: []model.PermissionEntry{{Path: "/教学", Permission: 256}}}
	if err := db.CreateRole(&role); err != nil {
		t.Fatal(err)
	}
	for _, item := range []model.SettingItem{
		{Key: conf.DefaultRole, Value: "104"},
		{Key: conf.AllowRegister, Value: "true"},
		{Key: conf.SSOAutoRegister, Value: "true"},
		{Key: conf.SSODefaultDir, Value: "/legacy-sso"},
		{Key: conf.LdapDefaultDir, Value: "/legacy-ldap"},
	} {
		if err := op.SaveSettingItem(&item); err != nil {
			t.Fatal(err)
		}
	}
	invoke := func(handler gin.HandlerFunc, body any) {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")
		handler(c)
		var resp struct {
			Code    int
			Message string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Code != 200 {
			t.Fatalf("handler failed: %s, %v", w.Body.String(), err)
		}
	}
	invoke(Register, RegisterReq{Username: "registered", Password: "test-password"})
	invoke(CreateUser, model.User{Username: "created", Password: "test-password", BasePath: "/custom", Role: model.Roles{104}})
	if _, err := autoRegister("sso", "provider-id", gorm.ErrRecordNotFound); err != nil {
		t.Fatal(err)
	}
	if _, err := ladpRegister("ldap"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"registered", "created", "sso", "ldap"} {
		u, err := db.GetUserByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if u.BasePath != "/" || len(u.Role) != 1 || u.Role[0] != 104 {
			t.Fatalf("%s: invalid browsing root or role: %+v", name, u)
		}
		u.BasePath = "/attempt-to-restore"
		invoke(UpdateUser, u)
		u, err = db.GetUserById(u.ID)
		if err != nil || u.BasePath != "/" {
			t.Fatalf("edit restored non-root path for %s: %+v, %v", name, u, err)
		}
	}
}
