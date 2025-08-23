package handles

import (
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

func ListMySessions(c *gin.Context) {
	user := c.MustGet("user").(*model.User)
	sessions, err := db.ListSessionsByUser(user.ID)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, sessions)
}

type EvictSessionReq struct {
	SessionID string `json:"session_id"`
}

func EvictMySession(c *gin.Context) {
	var req EvictSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if _, err := db.GetSession(user.ID, req.SessionID); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := db.MarkInactive(req.SessionID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func ListSessions(c *gin.Context) {
	sessions, err := db.ListSessions()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, sessions)
}

func EvictSession(c *gin.Context) {
	var req EvictSessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := db.MarkInactive(req.SessionID); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}
