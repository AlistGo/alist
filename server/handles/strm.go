package handles

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type GenerateStrmReq struct {
	Path string `json:"path" form:"path"`
}

// GenerateStrm enqueues a strm generation task for the given path (which must be
// inside a Strm storage). Admin only (route is under the admin group).
func GenerateStrm(c *gin.Context) {
	var req GenerateStrmReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	storage, actualPath, err := op.GetStorageAndActualPath(reqPath)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if _, ok := storage.(driver.StrmGenerator); !ok {
		common.ErrorStrResp(c, "not a strm storage", 400)
		return
	}
	t := &fs.StrmGenerateTask{
		TaskExtension:    task.TaskExtension{Creator: user},
		StorageMountPath: storage.GetStorage().MountPath,
		Path:             actualPath,
	}
	fs.StrmGenerateTaskManager.Add(t)
	common.SuccessResp(c, gin.H{"task": getTaskInfo(t)})
}
