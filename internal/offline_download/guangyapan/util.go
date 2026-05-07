package guangyapan

import (
	"context"
	"fmt"
	"time"

	"github.com/Xhofe/go-cache"
	guangyapandriver "github.com/alist-org/alist/v3/drivers/guangyapan"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/singleflight"
)

const (
	offlineStatusQueued             = 0
	offlineStatusRunning            = 1
	offlineStatusCompleted          = 2
	offlineStatusFailed             = 3
	offlineStatusCanceled           = 4
	offlineStatusPartiallyCompleted = 5
)

var taskCache = cache.NewMemCache(cache.WithShards[[]guangyapandriver.OfflineTask](16))
var taskG singleflight.Group[[]guangyapandriver.OfflineTask]

func (g *GuangYaPan) GetTasks(driver *guangyapandriver.GuangYaPan, taskID string) ([]guangyapandriver.OfflineTask, error) {
	key := op.Key(driver, "/cloudcollection/v1/list_task/"+taskID)
	if !g.refreshTaskCache {
		if tasks, ok := taskCache.Get(key); ok {
			return tasks, nil
		}
	}
	g.refreshTaskCache = false
	tasks, err, _ := taskG.Do(key, func() ([]guangyapandriver.OfflineTask, error) {
		ctx := context.Background()
		tasks, err := driver.OfflineList(ctx, []string{taskID}, nil, "", 0)
		if err != nil {
			return nil, err
		}
		if len(tasks) > 0 {
			taskCache.Set(key, tasks, cache.WithEx[[]guangyapandriver.OfflineTask](time.Second*10))
		} else {
			taskCache.Del(key)
		}
		return tasks, nil
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (g *GuangYaPan) DelTaskCache(driver *guangyapandriver.GuangYaPan, taskID string) {
	taskCache.Del(op.Key(driver, "/cloudcollection/v1/list_task/"+taskID))
}

func taskStatusText(task guangyapandriver.OfflineTask) string {
	switch task.Status {
	case offlineStatusQueued:
		return "queued"
	case offlineStatusRunning:
		if task.Progress > 0 {
			return fmt.Sprintf("running (%d%%)", task.Progress)
		}
		return "running"
	case offlineStatusCompleted:
		return "completed"
	case offlineStatusFailed:
		return "failed"
	case offlineStatusCanceled:
		return "canceled"
	case offlineStatusPartiallyCompleted:
		return "partially completed"
	default:
		return fmt.Sprintf("unknown status %d", task.Status)
	}
}
