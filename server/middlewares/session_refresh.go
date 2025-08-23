package middlewares

import (
	"github.com/alist-org/alist/v3/internal/device"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/gin-gonic/gin"
)

// SessionRefresh updates session's last_active after successful requests.
func SessionRefresh(c *gin.Context) {
	c.Next()
	if c.Writer.Status() >= 400 {
		return
	}
	if inactive, ok := c.Get("session_inactive"); ok {
		if b, ok := inactive.(bool); ok && b {
			return
		}
	}
	userVal, uok := c.Get("user")
	keyVal, kok := c.Get("device_key")
	if uok && kok {
		user := userVal.(*model.User)
		device.Refresh(user.ID, keyVal.(string))
	}
}
