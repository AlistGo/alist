package fs

import (
	"fmt"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
	"github.com/xhofe/tache"
)

type StrmGenerateTask struct {
	task.TaskExtension
	StorageMountPath string `json:"storage_mount_path"`
	Path             string `json:"path"` // actual path relative to the storage root
}

func (t *StrmGenerateTask) GetName() string {
	return fmt.Sprintf("generate strm [%s](%s)", t.StorageMountPath, t.Path)
}

func (t *StrmGenerateTask) GetStatus() string {
	return "generating strm"
}

func (t *StrmGenerateTask) Run() error {
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	storage, err := op.GetStorageByMountPath(t.StorageMountPath)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	gen, ok := storage.(driver.StrmGenerator)
	if !ok {
		return errors.New("not a strm storage")
	}
	return gen.GenerateLocal(t.Ctx(), t.Path, t.SetProgress)
}

var StrmGenerateTaskManager *tache.Manager[*StrmGenerateTask]
