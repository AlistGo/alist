package tool

import (
	"context"
	"fmt"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/xhofe/tache"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"time"
)

type TransferTask struct {
	task.TaskExtension
	Status       string        `json:"-"` //don't save status to save space
	SrcObjPath   string        `json:"src_obj_path"`
	DstDirPath   string        `json:"dst_dir_path"`
	SrcStorage   driver.Driver `json:"-"`
	DstStorage   driver.Driver `json:"-"`
	SrcStorageMp string        `json:"src_storage_mp"`
	DstStorageMp string        `json:"dst_storage_mp"`
	DeletePolicy DeletePolicy  `json:"delete_policy"`
}

func (t *TransferTask) Run() error {
	t.ClearEndTime()
	t.SetStartTime(time.Now())
	defer func() { t.SetEndTime(time.Now()) }()
	if t.SrcStorage == nil {
		return transferStdPath(t)
	} else {
		return transferObjPath(t)
	}
}

func (t *TransferTask) GetName() string {
	return fmt.Sprintf("transfer [%s](%s) to [%s](%s)", t.SrcStorageMp, t.SrcObjPath, t.DstStorageMp, t.DstDirPath)
}

func (t *TransferTask) GetStatus() string {
	return t.Status
}

func (t *TransferTask) OnSucceeded() {
	if t.DeletePolicy == DeleteOnUploadSucceed || t.DeletePolicy == DeleteAlways {
		if t.SrcStorage == nil {
			removeStdTemp(t)
		} else {
			removeObjTemp(t)
		}
	}
}

func (t *TransferTask) OnFailed() {
	if t.DeletePolicy == DeleteOnUploadFailed || t.DeletePolicy == DeleteAlways {
		if t.SrcStorage == nil {
			removeStdTemp(t)
		} else {
			removeObjTemp(t)
		}
	}
}

var (
	TransferTaskManager *tache.Manager[*TransferTask]
)

func transferStd(ctx context.Context, tempDir, dstDirPath string, deletePolicy DeletePolicy) error {
	dstStorage, dstDirActualPath, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return errors.WithMessage(err, "failed get dst storage")
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}
	taskCreator, _ := ctx.Value("user").(*model.User)
	for _, entry := range entries {
		t := &TransferTask{
			TaskExtension: task.TaskExtension{
				Creator: taskCreator,
			},
			SrcObjPath:   stdpath.Join(tempDir, entry.Name()),
			DstDirPath:   dstDirActualPath,
			DstStorage:   dstStorage,
			DstStorageMp: dstStorage.GetStorage().MountPath,
			DeletePolicy: deletePolicy,
		}
		TransferTaskManager.Add(t)
	}
	return nil
}

func transferStdPath(t *TransferTask) error {
	t.Status = "getting src object"
	info, err := os.Stat(t.SrcObjPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		t.Status = "src object is dir, listing objs"
		entries, err := os.ReadDir(t.SrcObjPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			srcRawPath := stdpath.Join(t.SrcObjPath, entry.Name())
			dstObjPath := stdpath.Join(t.DstDirPath, info.Name())
			t := &TransferTask{
				TaskExtension: task.TaskExtension{
					Creator: t.Creator,
				},
				SrcObjPath:   srcRawPath,
				DstDirPath:   dstObjPath,
				DstStorage:   t.DstStorage,
				SrcStorageMp: t.SrcStorageMp,
				DstStorageMp: t.DstStorageMp,
				DeletePolicy: t.DeletePolicy,
			}
			TransferTaskManager.Add(t)
		}
		t.Status = "src object is dir, added all transfer tasks of files"
		return nil
	}
	return transferStdFile(t)
}

func transferStdFile(t *TransferTask) error {
	rc, err := os.Open(t.SrcObjPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open file %s", t.SrcObjPath)
	}
	info, err := rc.Stat()
	if err != nil {
		return errors.Wrapf(err, "failed to get file %s", t.SrcObjPath)
	}
	mimetype := utils.GetMimeType(t.SrcObjPath)
	s := &stream.FileStream{
		Ctx: nil,
		Obj: &model.Object{
			Name:     filepath.Base(t.SrcObjPath),
			Size:     info.Size(),
			Modified: info.ModTime(),
			IsFolder: false,
		},
		Reader:   rc,
		Mimetype: mimetype,
		Closers:  utils.NewClosers(rc),
	}
	t.SetTotalBytes(info.Size())
	return op.Put(t.Ctx(), t.DstStorage, t.DstDirPath, s, t.SetProgress)
}

func removeStdTemp(t *TransferTask) {
	info, err := os.Stat(t.SrcObjPath)
	if err != nil || info.IsDir() {
		return
	}
	if err := os.Remove(t.SrcObjPath); err != nil {
		log.Errorf("failed to delete temp file %s, error: %s", t.SrcObjPath, err.Error())
	}
}

func transferObj(ctx context.Context, tempDir, dstDirPath string, deletePolicy DeletePolicy) error {
	srcStorage, srcObjActualPath, err := op.GetStorageAndActualPath(tempDir)
	if err != nil {
		return errors.WithMessage(err, "failed get src storage")
	}
	dstStorage, dstDirActualPath, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return errors.WithMessage(err, "failed get dst storage")
	}
	objs, err := op.List(ctx, srcStorage, srcObjActualPath, model.ListArgs{})
	if err != nil {
		return errors.WithMessagef(err, "failed list src [%s] objs", tempDir)
	}
	taskCreator, _ := ctx.Value("user").(*model.User) // taskCreator is nil when convert failed
	for _, obj := range objs {
		t := &TransferTask{
			TaskExtension: task.TaskExtension{
				Creator: taskCreator,
			},
			SrcObjPath:   stdpath.Join(srcObjActualPath, obj.GetName()),
			DstDirPath:   dstDirActualPath,
			SrcStorage:   srcStorage,
			DstStorage:   dstStorage,
			SrcStorageMp: srcStorage.GetStorage().MountPath,
			DstStorageMp: dstStorage.GetStorage().MountPath,
			DeletePolicy: deletePolicy,
		}
		TransferTaskManager.Add(t)
	}
	return nil
}

func transferObjPath(t *TransferTask) error {
	t.Status = "getting src object"
	srcObj, err := op.Get(t.Ctx(), t.SrcStorage, t.SrcObjPath)
	if err != nil {
		return errors.WithMessagef(err, "failed get src [%s] file", t.SrcObjPath)
	}
	if srcObj.IsDir() {
		t.Status = "src object is dir, listing objs"
		objs, err := op.List(t.Ctx(), t.SrcStorage, t.SrcObjPath, model.ListArgs{})
		if err != nil {
			return errors.WithMessagef(err, "failed list src [%s] objs", t.SrcObjPath)
		}
		for _, obj := range objs {
			if utils.IsCanceled(t.Ctx()) {
				return nil
			}
			srcObjPath := stdpath.Join(t.SrcObjPath, obj.GetName())
			dstObjPath := stdpath.Join(t.DstDirPath, srcObj.GetName())
			TransferTaskManager.Add(&TransferTask{
				TaskExtension: task.TaskExtension{
					Creator: t.Creator,
				},
				SrcObjPath:   srcObjPath,
				DstDirPath:   dstObjPath,
				SrcStorage:   t.SrcStorage,
				DstStorage:   t.DstStorage,
				SrcStorageMp: t.SrcStorageMp,
				DstStorageMp: t.DstStorageMp,
				DeletePolicy: t.DeletePolicy,
			})
		}
		t.Status = "src object is dir, added all transfer tasks of objs"
		return nil
	}
	return transferObjFile(t)
}

func transferObjFile(t *TransferTask) error {
	srcFile, err := op.Get(t.Ctx(), t.SrcStorage, t.SrcObjPath)
	if err != nil {
		return errors.WithMessagef(err, "failed get src [%s] file", t.SrcObjPath)
	}
	link, _, err := op.Link(t.Ctx(), t.SrcStorage, t.SrcObjPath, model.LinkArgs{
		Header: http.Header{},
	})
	if err != nil {
		return errors.WithMessagef(err, "failed get [%s] link", t.SrcObjPath)
	}
	fs := stream.FileStream{
		Obj: srcFile,
		Ctx: t.Ctx(),
	}
	// any link provided is seekable
	ss, err := stream.NewSeekableStream(fs, link)
	if err != nil {
		return errors.WithMessagef(err, "failed get [%s] stream", t.SrcObjPath)
	}
	t.SetTotalBytes(srcFile.GetSize())
	return op.Put(t.Ctx(), t.DstStorage, t.DstDirPath, ss, t.SetProgress)
}

func removeObjTemp(t *TransferTask) {
	srcObj, err := op.Get(t.Ctx(), t.SrcStorage, t.SrcObjPath)
	if err != nil || srcObj.IsDir() {
		return
	}
	if err := op.Remove(t.Ctx(), t.SrcStorage, t.SrcObjPath); err != nil {
		log.Errorf("failed to delete temp obj %s, error: %s", t.SrcObjPath, err.Error())
	}
}
