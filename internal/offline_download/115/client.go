package _115

import (
	"context"
	"fmt"

	"github.com/alist-org/alist/v3/drivers/115"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
)

type Cloud115 struct {
	refreshTaskCache bool
}

func (p *Cloud115) Name() string {
	return "115 Cloud"
}

func (p *Cloud115) Items() []model.SettingItem {
	return nil
}

func (p *Cloud115) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (p *Cloud115) Init() (string, error) {
	p.refreshTaskCache = false
	return "ok", nil
}

func (p *Cloud115) IsReady() bool {
	return true
}

func (p *Cloud115) AddURL(args *tool.AddUrlArgs) (string, error) {
	// 添加新任务刷新缓存
	p.refreshTaskCache = true
	// args.TempDir 已经被修改为了 DstDirPath
	storage, actualPath, err := op.GetStorageAndActualPath(args.TempDir)
	if err != nil {
		return "", err
	}
	driver115, ok := storage.(*_115.Pan115)
	if !ok {
		return "", fmt.Errorf("unsupported storage driver for offline download, only 115 Cloud is supported")
	}

	ctx := context.Background()
	parentDir, err := op.GetUnwrap(ctx, storage, actualPath)
	if err != nil {
		return "", err
	}

	hashs, err := driver115.OfflineDownload(ctx, []string{args.Url}, parentDir)
	if err != nil || len(hashs) < 1 {
		return "", fmt.Errorf("failed to add offline download task: %w", err)
	}

	return hashs[0], nil
}

func (p *Cloud115) Remove(task *tool.DownloadTask) error {
	storage, _, err := op.GetStorageAndActualPath(task.DstDirPath)
	if err != nil {
		return err
	}
	driver115, ok := storage.(*_115.Pan115)
	if !ok {
		return fmt.Errorf("unsupported storage driver for offline download, only 115 Cloud is supported")
	}

	ctx := context.Background()
	if err := driver115.DeleteOfflineTasks(ctx, []string{task.GID}, false); err != nil {
		return err
	}
	return nil
}

func (p *Cloud115) Status(task *tool.DownloadTask) (*tool.Status, error) {
	storage, _, err := op.GetStorageAndActualPath(task.DstDirPath)
	if err != nil {
		return nil, err
	}
	driver115, ok := storage.(*_115.Pan115)
	if !ok {
		return nil, fmt.Errorf("unsupported storage driver for offline download, only 115 Cloud is supported")
	}

	tasks, err := driver115.OfflineList(context.Background())
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
		if t.InfoHash == task.GID {
			s.Progress = t.Percent
			s.Status = t.GetStatus()
			s.Completed = t.IsDone()
			s.TotalBytes = t.Size
			if t.IsFailed() {
				s.Err = fmt.Errorf(t.GetStatus())
			}
			return s, nil
		}
	}
	s.Err = fmt.Errorf("the task has been deleted")
	return nil, nil
}

var _ tool.Tool = (*Cloud115)(nil)

func init() {
	tool.Tools.Add(&Cloud115{})
}
