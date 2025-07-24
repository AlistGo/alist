package handles

import (
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"strconv"
)

type DelLabelFileBinDingReq struct {
	FileName string `json:"file_name"`
	LabelId  string `json:"label_id"`
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
	var req op.CreateLabelFileBinDingReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.IsDir == true {
		common.ErrorStrResp(c, "Unable to bind folder", 400)
		return
	}
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	if err := op.CreateLabelFileBinDing(req, userObj.ID); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	} else {
		common.SuccessResp(c, gin.H{
			"msg": "添加成功！",
		})
	}
}

func DelLabelByFileName(c *gin.Context) {
	var req DelLabelFileBinDingReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	labelId, err := strconv.ParseUint(req.LabelId, 10, 64)
	if err != nil {
		common.ErrorResp(c, fmt.Errorf("invalid label ID '%s': %v", req.LabelId, err), 500, true)
		return
	}
	if err = db.DelLabelFileBinDingById(uint(labelId), userObj.ID, req.FileName); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func GetFileByLabel(c *gin.Context) {
	labelId := c.Query("label_id")
	if labelId == "" {
		common.ErrorResp(c, errors.New("file_name must not empty"), 400)
		return
	}
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	fileList, err := op.GetFileByLabel(userObj.ID, labelId)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, fileList)
}
