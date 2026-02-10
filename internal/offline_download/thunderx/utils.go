package thunderx

import (
	"context"
	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/drivers/thunderx"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/singleflight"
	"time"
)

var taskCache = cache.NewMemCache(cache.WithShards[[]thunderx.OfflineTask](16))
var taskG singleflight.Group[[]thunderx.OfflineTask]

func (t *ThunderX) GetTasks(thunderxDriver *thunderx.ThunderX) ([]thunderx.OfflineTask, error) {
	key := op.Key(thunderxDriver, "/drive/v1/task")
	if !t.refreshTaskCache {
		if tasks, ok := taskCache.Get(key); ok {
			return tasks, nil
		}
	}
	t.refreshTaskCache = false
	tasks, err, _ := taskG.Do(key, func() ([]thunderx.OfflineTask, error) {
		ctx := context.Background()
		phase := []string{"PHASE_TYPE_RUNNING", "PHASE_TYPE_ERROR", "PHASE_TYPE_PENDING", "PHASE_TYPE_COMPLETE"}
		tasks, err := thunderxDriver.OfflineList(ctx, "", phase)
		if err != nil {
			return nil, err
		}
		// 添加缓存 10s
		if len(tasks) > 0 {
			taskCache.Set(key, tasks, cache.WithEx[[]thunderx.OfflineTask](time.Second*10))
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
