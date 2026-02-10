package handles

import (
	"context"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/search"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type UpdateIndexReq struct {
	Paths    []string `json:"paths"`
	MaxDepth int      `json:"max_depth"`
	//IgnorePaths []string `json:"ignore_paths"`
}

func BuildIndex(c *gin.Context) {
	if search.Running() {
		common.ErrorStrResp(c, "index is running", 400)
		return
	}
	go func() {
		ctx := context.Background()
		err := search.Clear(ctx)
		if err != nil {
			log.Errorf("clear index error: %+v", err)
			return
		}
		err = search.BuildIndex(context.Background(), []string{"/"},
			setting.GetInt(conf.MaxIndexDepth, 20), true)
		if err != nil {
			log.Errorf("build index error: %+v", err)
		}
	}()
	common.SuccessResp(c)
}

func UpdateIndex(c *gin.Context) {
	var req UpdateIndexReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if search.Running() {
		common.ErrorStrResp(c, "index is running", 400)
		return
	}
	if !search.Config(c).AutoUpdate {
		common.ErrorStrResp(c, "update is not supported for current index", 400)
		return
	}
	go func() {
		ctx := context.Background()
		for _, path := range req.Paths {
			err := search.Del(ctx, path)
			if err != nil {
				log.Errorf("delete index on %s error: %+v", path, err)
				return
			}
		}
		err := search.BuildIndex(context.Background(), req.Paths, req.MaxDepth, false)
		if err != nil {
			log.Errorf("update index error: %+v", err)
		}
	}()
	common.SuccessResp(c)
}

func StopIndex(c *gin.Context) {
	quit := search.Quit.Load()
	if quit == nil {
		common.ErrorStrResp(c, "index is not running", 400)
		return
	}
	select {
	case *quit <- struct{}{}:
	default:
	}
	common.SuccessResp(c)
}

func ClearIndex(c *gin.Context) {
	if search.Running() {
		common.ErrorStrResp(c, "index is running", 400)
		return
	}
	search.Clear(c)
	search.WriteProgress(&model.IndexProgress{
		ObjCount:     0,
		IsDone:       true,
		LastDoneTime: nil,
		Error:        "",
	})
	common.SuccessResp(c)
}

func GetProgress(c *gin.Context) {
	progress, err := search.Progress()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, progress)
}
