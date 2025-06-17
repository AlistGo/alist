package handles

import (
	_115 "github.com/alist-org/alist/v3/drivers/115"
	"github.com/alist-org/alist/v3/drivers/pikpak"
	"github.com/alist-org/alist/v3/drivers/thunder"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

type SetAria2Req struct {
	Uri    string `json:"uri" form:"uri"`
	Secret string `json:"secret" form:"secret"`
}

func SetAria2(c *gin.Context) {
	var req SetAria2Req
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	items := []model.SettingItem{
		{Key: conf.Aria2Uri, Value: req.Uri, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
		{Key: conf.Aria2Secret, Value: req.Secret, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("aria2")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	version, err := _tool.Init()
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, version)
}

type SetQbittorrentReq struct {
	Url      string `json:"url" form:"url"`
	Seedtime string `json:"seedtime" form:"seedtime"`
}

func SetQbittorrent(c *gin.Context) {
	var req SetQbittorrentReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	items := []model.SettingItem{
		{Key: conf.QbittorrentUrl, Value: req.Url, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
		{Key: conf.QbittorrentSeedtime, Value: req.Seedtime, Type: conf.TypeNumber, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("qBittorrent")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if _, err := _tool.Init(); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, "ok")
}

type SetTransmissionReq struct {
	Uri      string `json:"uri" form:"uri"`
	Seedtime string `json:"seedtime" form:"seedtime"`
}

func SetTransmission(c *gin.Context) {
	var req SetTransmissionReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	items := []model.SettingItem{
		{Key: conf.TransmissionUri, Value: req.Uri, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
		{Key: conf.TransmissionSeedtime, Value: req.Seedtime, Type: conf.TypeNumber, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("Transmission")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if _, err := _tool.Init(); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, "ok")
}

type Set115Req struct {
	TempDir string `json:"temp_dir" form:"temp_dir"`
}

func Set115(c *gin.Context) {
	var req Set115Req
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.TempDir != "" {
		storage, _, err := op.GetStorageAndActualPath(req.TempDir)
		if err != nil {
			common.ErrorStrResp(c, "storage does not exists", 400)
			return
		}
		if storage.Config().CheckStatus && storage.GetStorage().Status != op.WORK {
			common.ErrorStrResp(c, "storage not init: "+storage.GetStorage().Status, 400)
			return
		}
		if _, ok := storage.(*_115.Pan115); !ok {
			common.ErrorStrResp(c, "unsupported storage driver for offline download, only 115 Cloud is supported", 400)
			return
		}
	}
	items := []model.SettingItem{
		{Key: conf.Pan115TempDir, Value: req.TempDir, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("115 Cloud")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if _, err := _tool.Init(); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, "ok")
}

type SetPikPakReq struct {
	TempDir string `json:"temp_dir" form:"temp_dir"`
}

func SetPikPak(c *gin.Context) {
	var req SetPikPakReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.TempDir != "" {
		storage, _, err := op.GetStorageAndActualPath(req.TempDir)
		if err != nil {
			common.ErrorStrResp(c, "storage does not exists", 400)
			return
		}
		if storage.Config().CheckStatus && storage.GetStorage().Status != op.WORK {
			common.ErrorStrResp(c, "storage not init: "+storage.GetStorage().Status, 400)
			return
		}
		if _, ok := storage.(*pikpak.PikPak); !ok {
			common.ErrorStrResp(c, "unsupported storage driver for offline download, only PikPak is supported", 400)
			return
		}
	}
	items := []model.SettingItem{
		{Key: conf.PikPakTempDir, Value: req.TempDir, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("PikPak")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if _, err := _tool.Init(); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, "ok")
}

type SetThunderReq struct {
	TempDir string `json:"temp_dir" form:"temp_dir"`
}

func SetThunder(c *gin.Context) {
	var req SetThunderReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.TempDir != "" {
		storage, _, err := op.GetStorageAndActualPath(req.TempDir)
		if err != nil {
			common.ErrorStrResp(c, "storage does not exists", 400)
			return
		}
		if storage.Config().CheckStatus && storage.GetStorage().Status != op.WORK {
			common.ErrorStrResp(c, "storage not init: "+storage.GetStorage().Status, 400)
			return
		}
		if _, ok := storage.(*thunder.Thunder); !ok {
			common.ErrorStrResp(c, "unsupported storage driver for offline download, only Thunder is supported", 400)
			return
		}
	}
	items := []model.SettingItem{
		{Key: conf.ThunderTempDir, Value: req.TempDir, Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
	if err := op.SaveSettingItems(items); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	_tool, err := tool.Tools.Get("Thunder")
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if _, err := _tool.Init(); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, "ok")
}

func OfflineDownloadTools(c *gin.Context) {
	tools := tool.Tools.Names()
	common.SuccessResp(c, tools)
}

type AddOfflineDownloadReq struct {
	Urls         []string `json:"urls"`
	Path         string   `json:"path"`
	Tool         string   `json:"tool"`
	DeletePolicy string   `json:"delete_policy"`
}

func AddOfflineDownload(c *gin.Context) {
	user := c.MustGet("user").(*model.User)
	if !user.CanAddOfflineDownloadTasks() {
		common.ErrorStrResp(c, "permission denied", 403)
		return
	}

	var req AddOfflineDownloadReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	var tasks []task.TaskExtensionInfo
	for _, url := range req.Urls {
		t, err := tool.AddURL(c, &tool.AddURLArgs{
			URL:          url,
			DstDirPath:   reqPath,
			Tool:         req.Tool,
			DeletePolicy: tool.DeletePolicy(req.DeletePolicy),
		})
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		if t != nil {
			tasks = append(tasks, t)
		}
	}
	common.SuccessResp(c, gin.H{
		"tasks": getTaskInfos(tasks),
	})
}
