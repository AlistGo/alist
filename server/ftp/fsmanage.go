package ftp

import (
	"context"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
	stdpath "path"
)

func Mkdir(ctx context.Context, path string) error {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return err
	}
	if !user.CanWrite() || !user.CanFTPManage() {
		meta, err := op.GetNearestMeta(stdpath.Dir(reqPath))
		if err != nil {
			if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
				return err
			}
		}
		if !common.CanWrite(meta, reqPath) {
			return errs.PermissionDenied
		}
	}
	return fs.MakeDir(ctx, reqPath)
}

func Remove(ctx context.Context, path string) error {
	user := ctx.Value("user").(*model.User)
	if !user.CanRemove() || !user.CanFTPManage() {
		return errs.PermissionDenied
	}
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return err
	}
	return fs.Remove(ctx, reqPath)
}

func Rename(ctx context.Context, oldPath, newPath string) error {
	user := ctx.Value("user").(*model.User)
	srcPath, err := user.JoinPath(oldPath)
	if err != nil {
		return err
	}
	dstPath, err := user.JoinPath(newPath)
	if err != nil {
		return err
	}
	srcDir, srcBase := stdpath.Split(srcPath)
	dstDir, dstBase := stdpath.Split(dstPath)
	if srcDir == dstDir {
		if !user.CanRename() || !user.CanFTPManage() {
			return errs.PermissionDenied
		}
		return fs.Rename(ctx, srcPath, dstBase)
	} else {
		if !user.CanFTPManage() || !user.CanMove() || (srcBase != dstBase && !user.CanRename()) {
			return errs.PermissionDenied
		}
		if err := fs.Move(ctx, srcPath, dstDir); err != nil {
			return err
		}
		if srcBase != dstBase {
			return fs.Rename(ctx, stdpath.Join(dstDir, srcBase), dstBase)
		}
		return nil
	}
}
