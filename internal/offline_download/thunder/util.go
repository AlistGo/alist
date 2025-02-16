package thunder

import (
	"context"
	"github.com/alist-org/alist/v3/internal/driver"
	"time"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/drivers/thunder"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/singleflight"
)

var taskCache = cache.NewMemCache(cache.WithShards[[]thunder.OfflineTask](16))
var taskG singleflight.Group[[]thunder.OfflineTask]

func (t *Thunder) GetTasks(storage driver.Driver) ([]thunder.OfflineTask, error) {
	key := op.Key(storage, "/drive/v1/task")
	if !t.refreshTaskCache {
		if tasks, ok := taskCache.Get(key); ok {
			return tasks, nil
		}
	}
	t.refreshTaskCache = false
	tasks, err, _ := taskG.Do(key, func() ([]thunder.OfflineTask, error) {
		var tasks []thunder.OfflineTask
		var err error
		if thunderDriver, ok := storage.(*thunder.Thunder); ok {
			tasks, err = thunderDriver.OfflineList(context.Background(), "")
		} else if expertDriver, ok := storage.(*thunder.ThunderExpert); ok {
			tasks, err = expertDriver.OfflineList(context.Background(), "")
		}
		if err != nil {
			return nil, err
		}
		// 添加缓存 10s
		if len(tasks) > 0 {
			taskCache.Set(key, tasks, cache.WithEx[[]thunder.OfflineTask](time.Second*10))
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
