package thunder_browser

import (
	"context"
	"errors"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/thunder_browser"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/setting"
	"strconv"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
)

type ThunderBrowser struct {
	refreshTaskCache bool
}

func (t *ThunderBrowser) Name() string {
	return "ThunderBrowser"
}

func (t *ThunderBrowser) Items() []model.SettingItem {
	return nil
}

func (t *ThunderBrowser) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (t *ThunderBrowser) Init() (string, error) {
	t.refreshTaskCache = false
	return "ok", nil
}

func (t *ThunderBrowser) IsReady() bool {
	tempDir := setting.GetStr(conf.ThunderBrowserTempDir)
	if tempDir == "" {
		return false
	}
	storage, _, err := op.GetStorageAndActualPath(tempDir)
	if err != nil {
		return false
	}

	switch storage.(type) {
	case *thunder_browser.ThunderBrowser, *thunder_browser.ThunderBrowserExpert:
		return true
	default:
		return false
	}
}

func (t *ThunderBrowser) AddURL(args *tool.AddUrlArgs) (string, error) {
	// 添加新任务刷新缓存
	t.refreshTaskCache = true
	storage, actualPath, err := op.GetStorageAndActualPath(args.TempDir)
	if err != nil {
		return "", err
	}

	ctx := context.Background()

	if err := op.MakeDir(ctx, storage, actualPath); err != nil {
		return "", err
	}

	parentDir, err := op.GetUnwrap(ctx, storage, actualPath)
	if err != nil {
		return "", err
	}

	var task *thunder_browser.OfflineTask
	switch v := storage.(type) {
	case *thunder_browser.ThunderBrowser:
		task, err = v.OfflineDownload(ctx, args.Url, parentDir, "")
	case *thunder_browser.ThunderBrowserExpert:
		task, err = v.OfflineDownload(ctx, args.Url, parentDir, "")
	default:
		return "", fmt.Errorf("unsupported storage driver for offline download, only ThunderBrowser is supported")
	}

	if err != nil {
		return "", fmt.Errorf("failed to add offline download task: %w", err)
	}

	if task == nil {
		return "", fmt.Errorf("failed to add offline download task: task is nil")
	}

	return task.ID, nil
}

func (t *ThunderBrowser) Remove(task *tool.DownloadTask) error {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return err
	}

	ctx := context.Background()

	switch v := storage.(type) {
	case *thunder_browser.ThunderBrowser:
		err = v.DeleteOfflineTasks(ctx, []string{task.GID}, false)
	case *thunder_browser.ThunderBrowserExpert:
		err = v.DeleteOfflineTasks(ctx, []string{task.GID}, false)
	default:
		return fmt.Errorf("unsupported storage driver for offline download, only ThunderBrowser is supported")
	}

	if err != nil {
		return err
	}
	return nil
}

func (t *ThunderBrowser) Status(task *tool.DownloadTask) (*tool.Status, error) {
	storage, _, err := op.GetStorageAndActualPath(task.TempDir)
	if err != nil {
		return nil, err
	}

	var tasks []thunder_browser.OfflineTask

	switch v := storage.(type) {
	case *thunder_browser.ThunderBrowser:
		tasks, err = t.GetTasks(v)
	case *thunder_browser.ThunderBrowserExpert:
		tasks, err = t.GetTasksExpert(v)
	default:
		return nil, fmt.Errorf("unsupported storage driver for offline download, only ThunderBrowser is supported")
	}

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

func init() {
	tool.Tools.Add(&ThunderBrowser{})
}
