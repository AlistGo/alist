package middlewares

import (
	"net/url"
	stdpath "path"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func FsUp(c *gin.Context) {
	path := c.GetHeader("File-Path")
	password := c.GetHeader("Password")
	path, err := url.PathUnescape(path)
	if err != nil {
		common.ErrorResp(c, err, 400)
		c.Abort()
		return
	}
	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(stdpath.Dir(path))
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			c.Abort()
			return
		}
	}
	perm := common.MergeRolePermissions(user, path)
	if !(common.CanAccessWithRoles(user, meta, path, password) &&
		(common.HasPermission(perm, common.PermWrite) || common.CanWrite(meta, stdpath.Dir(path)))) {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		c.Abort()
		return
	}
	c.Next()
}
