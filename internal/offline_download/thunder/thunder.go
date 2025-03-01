package thunder

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/thunder"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
)

type Thunder struct {
	refreshTaskCache bool
}

func (t *Thunder) Name() string {
	return "thunder"
}

func (t *Thunder) Items() []model.SettingItem {
	return nil
}

func (t *Thunder) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (t *Thunder) Init() (string, error) {
	t.refreshTaskCache = false
	return "ok", nil
}

func (t *Thunder) IsReady() bool {
	return true
}

func (t *Thunder) AddURL(args *tool.AddUrlArgs) (string, error) {
	// 添加新任务刷新缓存
	t.refreshTaskCache = true
	// args.TempDir 已经被修改为了 DstDirPath
	storage, actualPath, err := op.GetStorageAndActualPath(args.TempDir)
	if err != nil {
		return "", err
	}
	thunderDriver, ok := storage.(*thunder.Thunder)
	if !ok {
		return "", fmt.Errorf("unsupported storage driver for offline download, only Thunder is supported")
	}

	ctx := context.Background()
	parentDir, err := op.GetUnwrap(ctx, storage, actualPath)
	if err != nil {
		return "", err
	}

	task, err := thunderDriver.OfflineDownload(ctx, args.Url, parentDir, "")
	if err != nil {
		return "", fmt.Errorf("failed to add offline download task: %w", err)
	}

	return task.ID, nil
}

func (t *Thunder) Remove(task *tool.DownloadTask) error {
	storage, _, err := op.GetStorageAndActualPath(task.DstDirPath)
	if err != nil {
		return err
	}
	thunderDriver, ok := storage.(*thunder.Thunder)
	if !ok {
		return fmt.Errorf("unsupported storage driver for offline download, only Thunder is supported")
	}
	ctx := context.Background()
	err = thunderDriver.DeleteOfflineTasks(ctx, []string{task.GID}, false)
	if err != nil {
		return err
	}
	return nil
}

func (t *Thunder) Status(task *tool.DownloadTask) (*tool.Status, error) {
	storage, _, err := op.GetStorageAndActualPath(task.DstDirPath)
	if err != nil {
		return nil, err
	}
	thunderDriver, ok := storage.(*thunder.Thunder)
	if !ok {
		return nil, fmt.Errorf("unsupported storage driver for offline download, only Thunder is supported")
	}
	tasks, err := t.GetTasks(thunderDriver)
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
			s.Completed = (t.Phase == "PHASE_TYPE_COMPLETE")
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

func init() {
	tool.Tools.Add(&Thunder{})
}
