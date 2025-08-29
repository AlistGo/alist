package middlewares

import (
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/device"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// Auth is a middleware that checks if the user is logged in.
// if token is empty, set user to guest
func Auth(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if subtle.ConstantTimeCompare([]byte(token), []byte(setting.GetStr(conf.Token))) == 1 {
		admin, err := op.GetAdmin()
		if err != nil {
			common.ErrorResp(c, err, 500)
			c.Abort()
			return
		}
		if !HandleSession(c, admin) {
			return
		}
		log.Debugf("use admin token: %+v", admin)
		c.Next()
		return
	}
	if token == "" {
		guest, err := op.GetGuest()
		if err != nil {
			common.ErrorResp(c, err, 500)
			c.Abort()
			return
		}
		if guest.Disabled {
			common.ErrorStrResp(c, "Guest user is disabled, login please", 401)
			c.Abort()
			return
		}
		if len(guest.Role) > 0 {
			roles, err := op.GetRolesByUserID(guest.ID)
			if err != nil {
				common.ErrorStrResp(c, fmt.Sprintf("Fail to load guest roles: %v", err), 500)
				c.Abort()
				return
			}
			guest.RolesDetail = roles
		}
		if !HandleSession(c, guest) {
			return
		}
		log.Debugf("use empty token: %+v", guest)
		c.Next()
		return
	}
	userClaims, err := common.ParseToken(token)
	if err != nil {
		common.ErrorResp(c, err, 401)
		c.Abort()
		return
	}
	user, err := op.GetUserByName(userClaims.Username)
	if err != nil {
		common.ErrorResp(c, err, 401)
		c.Abort()
		return
	}
	// validate password timestamp
	if userClaims.PwdTS != user.PwdTS {
		common.ErrorStrResp(c, "Password has been changed, login please", 401)
		c.Abort()
		return
	}
	if user.Disabled {
		common.ErrorStrResp(c, "Current user is disabled, replace please", 401)
		c.Abort()
		return
	}
	if len(user.Role) > 0 {
		roles, err := op.GetRolesByUserID(user.ID)
		if err != nil {
			common.ErrorStrResp(c, fmt.Sprintf("Fail to load roles: %v", err), 500)
			c.Abort()
			return
		}
		user.RolesDetail = roles
	}
	if !HandleSession(c, user) {
		return
	}
	log.Debugf("use login token: %+v", user)
	c.Next()
}

// HandleSession verifies device sessions and stores context values.
func HandleSession(c *gin.Context, user *model.User) bool {
	clientID := c.GetHeader("Client-Id")
	if clientID == "" {
		clientID = c.Query("client_id")
	}
	key := utils.GetMD5EncodeStr(fmt.Sprintf("%d-%s", user.ID, clientID))
	if err := device.Handle(user.ID, key, c.Request.UserAgent(), c.ClientIP()); err != nil {
		token := c.GetHeader("Authorization")
		if errors.Is(err, errs.SessionInactive) {
			_ = common.InvalidateToken(token)
			common.ErrorResp(c, err, 401)
		} else {
			common.ErrorResp(c, err, 403)
		}
		c.Abort()
		return false
	}
	c.Set("device_key", key)
	c.Set("user", user)
	return true
}

func Authn(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if subtle.ConstantTimeCompare([]byte(token), []byte(setting.GetStr(conf.Token))) == 1 {
		admin, err := op.GetAdmin()
		if err != nil {
			common.ErrorResp(c, err, 500)
			c.Abort()
			return
		}
		c.Set("user", admin)
		log.Debugf("use admin token: %+v", admin)
		c.Next()
		return
	}
	if token == "" {
		guest, err := op.GetGuest()
		if err != nil {
			common.ErrorResp(c, err, 500)
			c.Abort()
			return
		}
		c.Set("user", guest)
		log.Debugf("use empty token: %+v", guest)
		c.Next()
		return
	}
	userClaims, err := common.ParseToken(token)
	if err != nil {
		common.ErrorResp(c, err, 401)
		c.Abort()
		return
	}
	user, err := op.GetUserByName(userClaims.Username)
	if err != nil {
		common.ErrorResp(c, err, 401)
		c.Abort()
		return
	}
	// validate password timestamp
	if userClaims.PwdTS != user.PwdTS {
		common.ErrorStrResp(c, "Password has been changed, login please", 401)
		c.Abort()
		return
	}
	if user.Disabled {
		common.ErrorStrResp(c, "Current user is disabled, replace please", 401)
		c.Abort()
		return
	}
	if len(user.Role) > 0 {
		var roles []model.Role
		for _, roleID := range user.Role {
			role, err := op.GetRole(uint(roleID))
			if err != nil {
				common.ErrorStrResp(c, fmt.Sprintf("load role %d failed", roleID), 500)
				c.Abort()
				return
			}
			roles = append(roles, *role)
		}
		user.RolesDetail = roles
	}
	c.Set("user", user)
	log.Debugf("use login token: %+v", user)
	c.Next()
}

func AuthNotGuest(c *gin.Context) {
	user := c.MustGet("user").(*model.User)
	if user.IsGuest() {
		common.ErrorStrResp(c, "You are a guest", 403)
		c.Abort()
	} else {
		c.Next()
	}
}

func AuthAdmin(c *gin.Context) {
	user := c.MustGet("user").(*model.User)
	if !user.IsAdmin() {
		common.ErrorStrResp(c, "You are not an admin", 403)
		c.Abort()
	} else {
		c.Next()
	}
}
