package handles

import (
	"errors"
	"fmt"

	"github.com/alist-org/alist/v3/internal/device"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

// activateDeviceOnLogin reactivates the device session of the client that is
// logging in and returns its key, as the password and LDAP logins do. Without
// it a client whose session was marked inactive by a logout still gets a token,
// but every later request is rejected with "session inactive".
// It answers the request itself when the session may not be activated.
func activateDeviceOnLogin(c *gin.Context, user *model.User) (string, bool) {
	clientID := c.GetHeader("Client-Id")
	if clientID == "" {
		clientID = c.Query("client_id")
	}
	key := utils.GetMD5EncodeStr(fmt.Sprintf("%d-%s", user.ID, clientID))
	if err := device.EnsureActiveOnLogin(user.ID, key, c.Request.UserAgent(), c.ClientIP()); err != nil {
		if errors.Is(err, errs.TooManyDevices) {
			common.ErrorResp(c, err, 403)
		} else {
			common.ErrorResp(c, err, 400, true)
		}
		return "", false
	}
	return key, true
}
