package handles

import (
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"net/url"
	"strconv"
	"strings"
)

type DelLabelFileBinDingReq struct {
	FileName string `json:"file_name"`
	LabelId  string `json:"label_id"`
}

type pageResp[T any] struct {
	Content []T   `json:"content"`
	Total   int64 `json:"total"`
}

type restoreLabelBindingsReq struct {
	KeepIDs  bool                     `json:"keep_ids"`
	Override bool                     `json:"override"`
	Bindings []model.LabelFileBinding `json:"bindings"`
}

func GetLabelByFileName(c *gin.Context) {
	fileName := c.Query("file_name")
	if fileName == "" {
		common.ErrorResp(c, errors.New("file_name must not empty"), 400)
		return
	}
	decodedFileName, err := url.QueryUnescape(fileName)
	if err != nil {
		common.ErrorResp(c, errors.New("invalid file_name"), 400)
		return
	}
	fmt.Println(">>> 原始 fileName:", fileName)
	fmt.Println(">>> 解码后 fileName:", decodedFileName)
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}
	labels, err := op.GetLabelByFileName(userObj.ID, decodedFileName)
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

func ListLabelFileBinding(c *gin.Context) {
	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}

	pageStr := c.DefaultQuery("page", "1")
	sizeStr := c.DefaultQuery("page_size", "50")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page <= 0 {
		page = 1
	}
	pageSize, err := strconv.Atoi(sizeStr)
	if err != nil || pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}

	fileName := c.Query("file_name")
	labelIDStr := c.Query("label_id")
	var labelIDs []uint
	if labelIDStr != "" {
		parts := strings.Split(labelIDStr, ",")
		for _, p := range parts {
			if p == "" {
				continue
			}
			id64, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64)
			if err != nil {
				common.ErrorResp(c, fmt.Errorf("invalid label_id '%s': %v", p, err), 400)
				return
			}
			labelIDs = append(labelIDs, uint(id64))
		}
	}

	list, total, err := db.ListLabelFileBinDing(userObj.ID, labelIDs, fileName, page, pageSize)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, pageResp[model.LabelFileBinding]{
		Content: list,
		Total:   total,
	})
}

func RestoreLabelFileBinding(c *gin.Context) {
	var req restoreLabelBindingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if len(req.Bindings) == 0 {
		common.ErrorStrResp(c, "empty bindings", 400)
		return
	}

	if u, ok := c.Value("user").(*model.User); ok {
		for i := range req.Bindings {
			if req.Bindings[i].UserId == 0 {
				req.Bindings[i].UserId = u.ID
			}
		}
	}

	for i := range req.Bindings {
		b := req.Bindings[i]
		if b.UserId == 0 || b.LabelId == 0 || strings.TrimSpace(b.FileName) == "" {
			common.ErrorStrResp(c, "invalid binding: user_id/label_id/file_name required", 400)
			return
		}
	}

	if err := op.RestoreLabelFileBindings(req.Bindings, req.KeepIDs, req.Override); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, gin.H{
		"msg": fmt.Sprintf("restored %d rows", len(req.Bindings)),
	})
}

func CreateLabelFileBinDingBatch(c *gin.Context) {
	var req struct {
		Items []op.CreateLabelFileBinDingReq `json:"items" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		common.ErrorResp(c, err, 400)
		return
	}

	userObj, ok := c.Value("user").(*model.User)
	if !ok {
		common.ErrorStrResp(c, "user invalid", 401)
		return
	}

	type perResult struct {
		Name   string `json:"name"`
		Ok     bool   `json:"ok"`
		ErrMsg string `json:"errMsg,omitempty"`
	}
	results := make([]perResult, 0, len(req.Items))
	succeed := 0

	for _, item := range req.Items {
		if item.IsDir {
			results = append(results, perResult{Name: item.Name, Ok: false, ErrMsg: "Unable to bind folder"})
			continue
		}
		if err := op.CreateLabelFileBinDing(item, userObj.ID); err != nil {
			results = append(results, perResult{Name: item.Name, Ok: false, ErrMsg: err.Error()})
			continue
		}
		succeed++
		results = append(results, perResult{Name: item.Name, Ok: true})
	}

	common.SuccessResp(c, gin.H{
		"total":   len(req.Items),
		"succeed": succeed,
		"failed":  len(req.Items) - succeed,
		"results": results,
	})
}
