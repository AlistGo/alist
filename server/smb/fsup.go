package smb

import (
	"context"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	stdpath "path"
	"time"

	"github.com/KirCute/go-smb2-alist/vfs"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
)

type writingFile struct {
	path string
	f    *os.File
}

func newUpload(ctx context.Context, path string) (*writingFile, error) {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = uploadAuth(ctx, reqPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tmpFile, err := os.CreateTemp(conf.Conf.TempDir, "file-*")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &writingFile{f: tmpFile, path: utils.FixAndCleanPath(path)}, nil
}

func uploadAuth(ctx context.Context, path string) error {
	user := ctx.Value("user").(*model.User)
	meta, err := op.GetNearestMeta(stdpath.Dir(path))
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return errors.WithStack(err)
		}
	}
	if !(common.CanAccess(user, meta, path, "") &&
		((user.CanSMBManage() && user.CanWrite()) || common.CanWrite(meta, stdpath.Dir(path)))) {
		return errors.WithStack(errs.PermissionDenied)
	}
	return nil
}

func (f *writingFile) close(ctx context.Context) (task.TaskExtensionInfo, error) {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(f.path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	dir, name := stdpath.Split(reqPath)
	size, err := f.f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if _, err := f.f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}
	arr := make([]byte, 512)
	if _, err := f.f.Read(arr); err != nil {
		return nil, errors.WithStack(err)
	}
	contentType := http.DetectContentType(arr)
	if _, err := f.f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.WithStack(err)
	}
	_ = fs.Remove(ctx, reqPath)
	s := &stream.FileStream{
		Obj: &model.Object{
			Name:     name,
			Size:     size,
			Modified: time.Now(),
		},
		Mimetype:     contentType,
		WebPutAsTask: true,
	}
	s.SetTmpFile(f.f)
	return fs.PutAsTask(ctx, dir, s)
}

func MakeTaskAttribute(tsk task.TaskExtensionInfo) (*vfs.Attributes, error) {
	a := &vfs.Attributes{}
	h := fnv.New64()
	_, err := h.Write([]byte(tsk.GetID()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a.SetInodeNumber(h.Sum64())
	a.SetSizeBytes(uint64(tsk.GetTotalBytes()))
	a.SetLastDataModificationTime(*tsk.GetStartTime())
	a.SetFileType(vfs.FileTypeRegularFile)
	a.SetUnixMode(0644)
	a.SetPermissions(vfs.NewPermissionsFromMode(0644))
	return a, nil
}
