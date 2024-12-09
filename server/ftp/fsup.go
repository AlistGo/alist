package ftp

import (
	"context"
	ftpserver "github.com/KirCute/ftpserverlib-pasvportmap"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"os"
	stdpath "path"
	"time"
)

type FileUploadProxy struct {
	ftpserver.FileTransfer
	buffer *os.File
	path   string
	ctx    context.Context
}

func OpenUpload(ctx context.Context, path string) (*FileUploadProxy, error) {
	user := ctx.Value("user").(*model.User)
	path, err := user.JoinPath(path)
	if err != nil {
		return nil, err
	}
	meta, err := op.GetNearestMeta(stdpath.Dir(path))
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return nil, err
		}
	}
	if !(common.CanAccess(user, meta, path, ctx.Value("meta_pass").(string)) &&
		((user.CanFTPManage() && user.CanWrite()) || common.CanWrite(meta, stdpath.Dir(path)))) {
		return nil, errs.PermissionDenied
	}
	tmpFile, err := os.CreateTemp(conf.Conf.TempDir, "file-*")
	if err != nil {
		return nil, err
	}
	return &FileUploadProxy{buffer: tmpFile, path: path, ctx: ctx}, nil
}

func (f *FileUploadProxy) Read(p []byte) (n int, err error) {
	return 0, errs.NotSupport
}

func (f *FileUploadProxy) Write(p []byte) (n int, err error) {
	return f.buffer.Write(p)
}

func (f *FileUploadProxy) Seek(offset int64, whence int) (int64, error) {
	return 0, errs.NotSupport
}

func (f *FileUploadProxy) Close() error {
	dir, name := stdpath.Split(f.path)
	size, err := f.buffer.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := f.buffer.Seek(0, io.SeekStart); err != nil {
		return err
	}
	arr := make([]byte, 512)
	if _, err := f.buffer.Read(arr); err != nil {
		return err
	}
	contentType := http.DetectContentType(arr)
	if _, err := f.buffer.Seek(0, io.SeekStart); err != nil {
		return err
	}
	s := &stream.FileStream{
		Obj: &model.Object{
			Name:     name,
			Size:     size,
			Modified: time.Now(),
		},
		Mimetype:     contentType,
		WebPutAsTask: false,
	}
	s.SetTmpFile(f.buffer)
	return fs.PutDirectly(f.ctx, dir, s, true)
}
