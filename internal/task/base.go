package task

import (
	"context"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/xhofe/tache"
	"sync"
	"time"
)

type TaskExtension struct {
	tache.Base
	ctx          context.Context
	ctxInitMutex sync.Mutex
	Creator      *model.User
	startTime    *time.Time
	endTime      *time.Time
	totalBytes   int64
}

func (t *TaskExtension) SetCreator(creator *model.User) {
	t.Creator = creator
	t.Persist()
}

func (t *TaskExtension) GetCreator() *model.User {
	return t.Creator
}

func (t *TaskExtension) SetStartTime(startTime time.Time) {
	t.startTime = &startTime
}

func (t *TaskExtension) GetStartTime() *time.Time {
	return t.startTime
}

func (t *TaskExtension) SetEndTime(endTime time.Time) {
	t.endTime = &endTime
}

func (t *TaskExtension) GetEndTime() *time.Time {
	return t.endTime
}

func (t *TaskExtension) ClearEndTime() {
	t.endTime = nil
}

func (t *TaskExtension) SetTotalBytes(totalBytes int64) {
	t.totalBytes = totalBytes
}

func (t *TaskExtension) GetTotalBytes() int64 {
	return t.totalBytes
}

func (t *TaskExtension) Ctx() context.Context {
	if t.ctx == nil {
		t.ctxInitMutex.Lock()
		if t.ctx == nil {
			t.ctx = context.WithValue(t.Base.Ctx(), "user", t.Creator)
		}
		t.ctxInitMutex.Unlock()
	}
	return t.ctx
}

func (t *TaskExtension) ReinitCtx() {
	if !conf.Conf.Tasks.AllowRetryCanceled {
		return
	}
	select {
	case <-t.Base.Ctx().Done():
		ctx, cancel := context.WithCancel(context.Background())
		t.SetCtx(ctx)
		t.SetCancelFunc(cancel)
		t.ctx = nil
	default:
	}
}

type TaskExtensionInfo interface {
	tache.TaskWithInfo
	GetCreator() *model.User
	GetStartTime() *time.Time
	GetEndTime() *time.Time
	GetTotalBytes() int64
}
