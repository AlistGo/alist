package handles

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/device"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestSSOLoginReactivatesInactiveDeviceSession covers the login flow of the
// generic SSO providers: after a logout the device session of a client is
// marked inactive, and the login path has to activate it again, as the
// password and LDAP logins do (see device.EnsureActiveOnLogin).
func TestSSOLoginReactivatesInactiveDeviceSession(t *testing.T) {
	oldConf := conf.Conf
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sso.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { op.ClearUserCache(); op.SettingCacheUpdate(); sqlDB.Close(); conf.Conf = oldConf })

	if err := db.CreateRole(&model.Role{ID: 104, Name: "sso-role"}); err != nil {
		t.Fatal(err)
	}

	ssoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/login/oauth/access_token":
			fmt.Fprint(w, `{"access_token":"test-access"}`)
		case "/api/userinfo":
			fmt.Fprint(w, `{"sub":"provider-1","preferred_username":"sso-user"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ssoServer.Close()

	for _, item := range []model.SettingItem{
		{Key: conf.DefaultRole, Value: "104"},
		{Key: conf.SSOLoginEnabled, Value: "true"},
		{Key: conf.SSOLoginPlatform, Value: "Casdoor"},
		{Key: conf.SSOEndpointName, Value: ssoServer.URL},
		{Key: conf.SSOClientId, Value: "test-client"},
		{Key: conf.SSOClientSecret, Value: "test-secret"},
	} {
		if err := op.SaveSettingItem(&item); err != nil {
			t.Fatal(err)
		}
	}

	user := &model.User{Username: "sso-user", Password: "test-password", SsoID: "provider-1", BasePath: "/", Role: model.Roles{104}}
	if err := op.CreateUser(user); err != nil {
		t.Fatal(err)
	}

	// The client logs out: its device session is kept, but marked inactive.
	clientID := "device-of-the-client"
	deviceKey := utils.GetMD5EncodeStr(fmt.Sprintf("%d-%s", user.ID, clientID))
	if err := db.CreateSession(&model.Session{
		UserID:     user.ID,
		DeviceKey:  deviceKey,
		UserAgent:  "test-agent",
		IP:         "127.0.0.1",
		LastActive: time.Now().Unix(),
		Status:     model.SessionActive,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkInactive(deviceKey); err != nil {
		t.Fatal(err)
	}
	if err := device.Handle(user.ID, deviceKey, "test-agent", "127.0.0.1"); !errors.Is(err, errs.SessionInactive) {
		t.Fatalf("precondition: an inactive session must reject requests, got %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/auth/sso_callback?method=sso_get_token&code=test-code", nil)
	c.Request.Header.Set("Client-Id", clientID)
	SSOLoginCallback(c)

	body := w.Body.String()
	if !strings.Contains(body, "device_key") {
		t.Fatalf("the SSO login answered without device_key, so the client cannot keep its session active (body: %s)", body)
	}
	if !strings.Contains(body, "token") {
		t.Fatalf("the SSO login answered without a token (body: %s)", body)
	}

	sess, err := db.GetSession(user.ID, deviceKey)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != model.SessionActive {
		t.Fatalf("the SSO login left the device session inactive (status=%d), so every later request gets 401 session inactive", sess.Status)
	}
	if err := device.Handle(user.ID, deviceKey, "test-agent", "127.0.0.1"); err != nil {
		t.Fatalf("a request right after the SSO login is still rejected: %v", err)
	}
}
