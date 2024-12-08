package server

import (
	"context"
	"crypto/subtle"
	"net/http"
	"path"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/webdav"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

var handler *webdav.Handler

func WebDav(dav *gin.RouterGroup) {
	handler = &webdav.Handler{
		Prefix:     path.Join(conf.URL.Path, "/dav"),
		LockSystem: webdav.NewMemLS(),
		Logger: func(request *http.Request, err error) {
			log.Errorf("%s %s %+v", request.Method, request.URL.Path, err)
		},
	}
	dav.Use(WebDAVAuth)
	dav.Any("/*path", ServeWebDAV)
	dav.Any("", ServeWebDAV)
	dav.Handle("PROPFIND", "/*path", ServeWebDAV)
	dav.Handle("PROPFIND", "", ServeWebDAV)
	dav.Handle("MKCOL", "/*path", ServeWebDAV)
	dav.Handle("LOCK", "/*path", ServeWebDAV)
	dav.Handle("UNLOCK", "/*path", ServeWebDAV)
	dav.Handle("PROPPATCH", "/*path", ServeWebDAV)
	dav.Handle("COPY", "/*path", ServeWebDAV)
	dav.Handle("MOVE", "/*path", ServeWebDAV)
}

func ServeWebDAV(c *gin.Context) {
	user := c.MustGet("user").(*model.User)
	ctx := context.WithValue(c.Request.Context(), "user", user)
	handler.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
}

func WebDAVAuth(c *gin.Context) {
	guest, _ := op.GetGuest()
	username, password, ok := c.Request.BasicAuth()
	if !ok {
		bt := c.GetHeader("Authorization")
		log.Debugf("[webdav auth] token: %s", bt)
		if strings.HasPrefix(bt, "Bearer") {
			bt = strings.TrimPrefix(bt, "Bearer ")
			token := setting.GetStr(conf.Token)
			if token != "" && subtle.ConstantTimeCompare([]byte(bt), []byte(token)) == 1 {
				admin, err := op.GetAdmin()
				if err != nil {
					log.Errorf("[webdav auth] failed get admin user: %+v", err)
					c.Status(http.StatusInternalServerError)
					c.Abort()
					return
				}
				c.Set("user", admin)
				c.Next()
				return
			}
		}
		if c.Request.Method == "OPTIONS" {
			c.Set("user", guest)
			c.Next()
			return
		}
		c.Writer.Header()["WWW-Authenticate"] = []string{`Basic realm="alist"`}
		c.Status(http.StatusUnauthorized)
		c.Abort()
		return
	}
	user, err := op.GetUserByName(username)
	if err != nil || user.IsOtpEnabled() || user.ValidateRawPassword(password) != nil {
		if c.Request.Method == "OPTIONS" {
			c.Set("user", guest)
			c.Next()
			return
		}
		c.Status(http.StatusUnauthorized)
		c.Abort()
		return
	}
	if user.Disabled || !user.CanWebdavRead() {
		if c.Request.Method == "OPTIONS" {
			c.Set("user", guest)
			c.Next()
			return
		}
		c.Status(http.StatusForbidden)
		c.Abort()
		return
	}
	if !user.CanWebdavManage() && utils.SliceContains([]string{"PUT", "DELETE", "PROPPATCH", "MKCOL", "COPY", "MOVE"}, c.Request.Method) {
		if c.Request.Method == "OPTIONS" {
			c.Set("user", guest)
			c.Next()
			return
		}
		c.Status(http.StatusForbidden)
		c.Abort()
		return
	}
	c.Set("user", user)
	c.Next()
}
