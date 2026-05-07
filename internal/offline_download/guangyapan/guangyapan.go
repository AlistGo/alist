package guangyapan

import (
	"context"
	"errors"
	"fmt"

	guangyapandriver "github.com/alist-org/alist/v3/drivers/guangyapan"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
)

type GuangYaPan struct {
	refreshTaskCache bool
}

func (g *GuangYaPan) Name() string {
	return "GuangYaPan"
}

func (g *GuangYaPan) Items() []model.SettingItem {
	return nil
}

func (g *GuangYaPan) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (g *GuangYaPan) Init() (string, error) {
	g.refreshTaskCache = false
	return "ok", nil
}

func (g *GuangYaPan) IsReady() bool {
	tempDir := setting.GetStr(conf.GuangYaPanTempDir)
	if tempDir == "" {
		return false
	}
	storage, _, err := op.GetStorageAndActualPath(tempDir)
	if err != nil {
		return false
	}
	if _, ok := storage.(*guangyapandriver.GuangYaPan); !ok {
		return false
	}
	return true
}

func (g *GuangYaPan) AddURL(args *tool.AddUrlArgs) (string, error) {
	g.refreshTaskCache = true
	storage, actualPath, err := op.GetStorageAndActualPath(args.TempDir)
	if err != nil {
		return "", err
	}
	driver, ok := storage.(*guangyapandriver.GuangYaPan)
	if !ok {
		return "", errors.New("GuangYaPan offline download only supports GuangYaPan destination storage")
	}

	ctx := context.Background()
	if err := op.MakeDir(ctx, storage, actualPath); err != nil {
		return "", err
	}
	parentDir, err := op.GetUnwrap(ctx, storage, actualPath)
	if err != nil {
		return "", err
	}
	task, err := driver.OfflineDownload(ctx, args.Url, parentDir, "")
	if err != nil {
		return "", fmt.Errorf("failed to add offline download task: %w", err)
	}
	return task.TaskID, nil
}

func (g *GuangYaPan) Remove(task *tool.DownloadTask) error {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return err
	}
	driver, ok := storage.(*guangyapandriver.GuangYaPan)
	if !ok {
		return errors.New("GuangYaPan offline download only supports GuangYaPan destination storage")
	}
	ctx := context.Background()
	if err := driver.DeleteOfflineTasks(ctx, []string{task.GID}, false); err != nil {
		return err
	}
	g.DelTaskCache(driver, task.GID)
	return nil
}

func (g *GuangYaPan) Status(task *tool.DownloadTask) (*tool.Status, error) {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return nil, err
	}
	driver, ok := storage.(*guangyapandriver.GuangYaPan)
	if !ok {
		return nil, errors.New("GuangYaPan offline download only supports GuangYaPan destination storage")
	}
	tasks, err := g.GetTasks(driver, task.GID)
	if err != nil {
		return nil, err
	}
	status := &tool.Status{
		Status: "the task has been deleted",
	}
	for _, t := range tasks {
		if t.TaskID != task.GID {
			continue
		}
		status.Progress = float64(t.Progress)
		status.TotalBytes = t.TotalSize
		status.Completed = t.Status == offlineStatusCompleted || t.Status == offlineStatusPartiallyCompleted
		status.Status = taskStatusText(t)
		if t.Status == offlineStatusFailed || t.Status == offlineStatusCanceled {
			status.Err = errors.New(status.Status)
		}
		return status, nil
	}
	status.Err = errors.New("the task has been deleted")
	return status, nil
}

func init() {
	tool.Tools.Add(&GuangYaPan{})
}
