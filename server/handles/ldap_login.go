package handles

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"gopkg.in/ldap.v3"
)

func LoginLdap(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	loginLdap(c, &req)
}

func loginLdap(c *gin.Context, req *LoginReq) {
	enabled := setting.GetBool(conf.LdapLoginEnabled)
	if !enabled {
		common.ErrorStrResp(c, "ldap is not enabled", 403)
		return
	}

	// check count of login
	ip := c.ClientIP()
	count, ok := loginCache.Get(ip)
	if ok && count >= defaultTimes {
		common.ErrorStrResp(c, "Too many unsuccessful sign-in attempts have been made using an incorrect username or password, Try again later.", 429)
		loginCache.Expire(ip, defaultDuration)
		return
	}

	// Auth start
	ldapServer := setting.GetStr(conf.LdapServer)
	ldapManagerDN := setting.GetStr(conf.LdapManagerDN)
	ldapManagerPassword := setting.GetStr(conf.LdapManagerPassword)
	ldapUserSearchBase := setting.GetStr(conf.LdapUserSearchBase)
	ldapUserSearchFilter := setting.GetStr(conf.LdapUserSearchFilter) // (uid=%s)

	// Connect to LdapServer
	l, err := dial(ldapServer)
	if err != nil {
		utils.Log.Errorf("failed to connect to LDAP: %v", err)
		common.ErrorResp(c, err, 500)
		return
	}

	// First bind with a read only user
	if ldapManagerDN != "" && ldapManagerPassword != "" {
		err = l.Bind(ldapManagerDN, ldapManagerPassword)
		if err != nil {
			utils.Log.Errorf("Failed to bind to LDAP: %v", err)
			common.ErrorResp(c, err, 500)
			return
		}
	}

	// Search for the given username
	searchRequest := ldap.NewSearchRequest(
		ldapUserSearchBase,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(ldapUserSearchFilter, req.Username),
		[]string{"dn"},
		nil,
	)
	sr, err := l.Search(searchRequest)
	if err != nil {
		utils.Log.Errorf("LDAP search failed: %v", err)
		common.ErrorResp(c, err, 500)
		return
	}
	if len(sr.Entries) != 1 {
		utils.Log.Errorf("User does not exist or too many entries returned")
		common.ErrorResp(c, err, 500)
		return
	}
	userDN := sr.Entries[0].DN

	// Bind as the user to verify their password
	err = l.Bind(userDN, req.Password)
	if err != nil {
		utils.Log.Errorf("Failed to auth. %v", err)
		common.ErrorResp(c, err, 400)
		loginCache.Set(ip, count+1)
		return
	} else {
		utils.Log.Infof("Auth successful username:%s", req.Username)
	}
	// Auth finished

	user, err := op.GetUserByName(req.Username)
	if err != nil {
		user, err = ladpRegister(req.Username)
		if err != nil {
			common.ErrorResp(c, err, 400)
			loginCache.Set(ip, count+1)
			return
		}
	}

	// generate token
	token, err := common.GenerateToken(user)
	if err != nil {
		common.ErrorResp(c, err, 400, true)
		return
	}
	common.SuccessResp(c, gin.H{"token": token})
	loginCache.Del(ip)
}

func ladpRegister(username string) (*model.User, error) {
	if username == "" {
		return nil, errors.New("cannot get username from ldap provider")
	}
	user := &model.User{
		ID:         0,
		Username:   username,
		Password:   random.String(16),
		Permission: int32(setting.GetInt(conf.LdapDefaultPermission, 0)),
		BasePath:   setting.GetStr(conf.LdapDefaultDir),
		Role:       nil,
		Disabled:   false,
	}
	if err := db.CreateUser(user); err != nil {
		return nil, err
	}
	return user, nil
}

func dial(ldapServer string) (*ldap.Conn, error) {
	var tlsEnabled bool = false
	if strings.HasPrefix(ldapServer, "ldaps://") {
		tlsEnabled = true
		ldapServer = strings.TrimPrefix(ldapServer, "ldaps://")
	} else if strings.HasPrefix(ldapServer, "ldap://") {
		ldapServer = strings.TrimPrefix(ldapServer, "ldap://")
	}

	if tlsEnabled {
		return ldap.DialTLS("tcp", ldapServer, &tls.Config{InsecureSkipVerify: true})
	} else {
		return ldap.Dial("tcp", ldapServer)
	}
}
