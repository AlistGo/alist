package handles

import (
	"errors"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type CreateLabelFileBinDingReq struct {
	FileName string `json:"file_name"`
	LabelIds string `json:"label_ids"`
}

func GetLabelByFileName(c *gin.Context) {
	fileName := c.Query("file_name")
	if fileName == "" {
		common.ErrorResp(c, errors.New("file_name must not empty"), 400)
		return
	}
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	labels, err := op.GetLabelByFileName(userObj.ID, fileName)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, labels)
}

func CreateLabelFileBinDing(c *gin.Context) {
	var req CreateLabelFileBinDingReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	if err := op.CreateLabelFileBinDing(req.LabelIds, req.FileName, userObj.ID); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	} else {
		common.SuccessResp(c, gin.H{
			"msg": "添加成功！",
		})
	}
}
