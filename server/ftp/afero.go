package ftp

import (
	"context"
	"errors"
	ftpserver "github.com/KirCute/ftpserverlib-pasvportmap"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/spf13/afero"
	"os"
	"time"
)

type AferoAdapter struct {
	ctx context.Context
}

func NewAferoAdapter(ctx context.Context) *AferoAdapter {
	return &AferoAdapter{ctx: ctx}
}

func (a *AferoAdapter) Create(_ string) (afero.File, error) {
	// See also GetHandle
	return nil, errs.NotImplement
}

func (a *AferoAdapter) Mkdir(name string, _ os.FileMode) error {
	return Mkdir(a.ctx, name)
}

func (a *AferoAdapter) MkdirAll(path string, perm os.FileMode) error {
	return a.Mkdir(path, perm)
}

func (a *AferoAdapter) Open(_ string) (afero.File, error) {
	// See also GetHandle and ReadDir
	return nil, errs.NotImplement
}

func (a *AferoAdapter) OpenFile(_ string, _ int, _ os.FileMode) (afero.File, error) {
	// See also GetHandle
	return nil, errs.NotImplement
}

func (a *AferoAdapter) Remove(name string) error {
	return Remove(a.ctx, name)
}

func (a *AferoAdapter) RemoveAll(path string) error {
	return a.Remove(path)
}

func (a *AferoAdapter) Rename(oldName, newName string) error {
	return Rename(a.ctx, oldName, newName)
}

func (a *AferoAdapter) Stat(name string) (os.FileInfo, error) {
	return Stat(a.ctx, name)
}

func (a *AferoAdapter) Name() string {
	return "AList FTP Endpoint"
}

func (a *AferoAdapter) Chmod(_ string, _ os.FileMode) error {
	return errs.NotSupport
}

func (a *AferoAdapter) Chown(_ string, _, _ int) error {
	return errs.NotSupport
}

func (a *AferoAdapter) Chtimes(_ string, _ time.Time, _ time.Time) error {
	return errs.NotSupport
}

func (a *AferoAdapter) ReadDir(name string) ([]os.FileInfo, error) {
	return List(a.ctx, name)
}

func (a *AferoAdapter) GetHandle(name string, flags int, offset int64) (ftpserver.FileTransfer, error) {
	if offset != 0 {
		return nil, errors.New("offset")
	}
	if (flags & os.O_APPEND) > 0 {
		return nil, errors.New("append")
	}
	if (flags & os.O_WRONLY) > 0 {
		return OpenUpload(a.ctx, name)
	}
	return OpenDownload(a.ctx, name)
}
