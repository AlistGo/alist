package thunderx

import (
	"context"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/thunderx"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"strconv"
)

type ThunderX struct {
	refreshTaskCache bool
}

func (t *ThunderX) Name() string {
	return "ThunderX"
}

func (t *ThunderX) Items() []model.SettingItem {
	return nil
}

func (t *ThunderX) Init() (string, error) {
	t.refreshTaskCache = false
	return "ok", nil
}

func (t *ThunderX) IsReady() bool {
	tempDir := setting.GetStr(conf.ThunderXTempDir)
	if tempDir == "" {
		return false
	}
	storage, _, err := op.GetStorageAndActualPath(tempDir)
	if err != nil {
		return false
	}
	if _, ok := storage.(*thunderx.ThunderX); !ok {
		return false
	}
	return true
}

func (t *ThunderX) AddURL(args *tool.AddUrlArgs) (string, error) {
	// 添加新任务刷新缓存
	t.refreshTaskCache = true
	storage, actualPath, err := op.GetStorageAndActualPath(args.TempDir)
	if err != nil {
		return "", err
	}
	thunderXDriver, ok := storage.(*thunderx.ThunderX)
	if !ok {
		return "", fmt.Errorf("unsupported storage driver for offline download, only ThunderX is supported")
	}

	ctx := context.Background()

	if err := op.MakeDir(ctx, storage, actualPath); err != nil {
		return "", err
	}

	parentDir, err := op.GetUnwrap(ctx, storage, actualPath)
	if err != nil {
		return "", err
	}

	task, err := thunderXDriver.OfflineDownload(ctx, args.Url, parentDir, "")
	if err != nil {
		return "", fmt.Errorf("failed to add offline download task: %w", err)
	}

	return task.ID, nil
}

func (t *ThunderX) Remove(task *tool.DownloadTask) error {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return err
	}
	thunderXDriver, ok := storage.(*thunderx.ThunderX)
	if !ok {
		return fmt.Errorf("unsupported storage driver for offline download, only ThunderX is supported")
	}
	ctx := context.Background()
	err = thunderXDriver.DeleteOfflineTasks(ctx, []string{task.GID}, false)
	if err != nil {
		return err
	}
	return nil
}

func (t *ThunderX) Status(task *tool.DownloadTask) (*tool.Status, error) {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return nil, err
	}
	thunderXDriver, ok := storage.(*thunderx.ThunderX)
	if !ok {
		return nil, fmt.Errorf("unsupported storage driver for offline download, only ThunderX is supported")
	}
	tasks, err := t.GetTasks(thunderXDriver)
	if err != nil {
		return nil, err
	}
	s := &tool.Status{
		Progress:  0,
		NewGID:    "",
		Completed: false,
		Status:    "the task has been deleted",
		Err:       nil,
	}
	for _, t := range tasks {
		if t.ID == task.GID {
			s.Progress = float64(t.Progress)
			s.Status = t.Message
			s.Completed = t.Phase == "PHASE_TYPE_COMPLETE"
			s.TotalBytes, err = strconv.ParseInt(t.FileSize, 10, 64)
			if err != nil {
				s.TotalBytes = 0
			}
			if t.Phase == "PHASE_TYPE_ERROR" {
				s.Err = errors.New(t.Message)
			}
			return s, nil
		}
	}
	s.Err = fmt.Errorf("the task has been deleted")
	return s, nil
}

func (t *ThunderX) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func init() {
	tool.Tools.Add(&ThunderX{})
}
