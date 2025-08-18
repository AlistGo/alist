package handles

import (
	"strconv"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func ListRoles(c *gin.Context) {
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	log.Debugf("%+v", req)
	roles, total, err := op.GetRoles(req.Page, req.PerPage)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, common.PageResp{Content: roles, Total: total})
}

func GetRole(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	role, err := op.GetRole(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, role)
}

func CreateRole(c *gin.Context) {
	var req model.Role
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.CreateRole(&req); err != nil {
		common.ErrorResp(c, err, 500, true)
	} else {
		common.SuccessResp(c)
	}
}

func UpdateRole(c *gin.Context) {
	var req struct {
		ID               uint                    `json:"id"`
		Name             string                  `json:"name" binding:"required"`
		Description      string                  `json:"description"`
		PermissionScopes []model.PermissionEntry `json:"permission_scopes"`
		Default          *bool                   `json:"default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	role, err := op.GetRole(req.ID)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	switch role.Name {
	case "admin":
		common.ErrorResp(c, errs.ErrChangeDefaultRole, 403)
		return

	case "guest":
		req.Name = "guest"
	}
	role.Name = req.Name
	role.Description = req.Description
	role.PermissionScopes = req.PermissionScopes
	if req.Default != nil {
		role.Default = *req.Default
	}
	if err := op.UpdateRole(role); err != nil {
		common.ErrorResp(c, err, 500, true)
	} else {
		common.SuccessResp(c)
	}
}

func DeleteRole(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	role, err := op.GetRole(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	if role.Name == "admin" || role.Name == "guest" {
		common.ErrorResp(c, errs.ErrChangeDefaultRole, 403)
		return
	}
	if err := op.DeleteRole(uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}
