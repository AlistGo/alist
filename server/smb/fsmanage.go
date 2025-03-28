package smb

import (
	"context"
	stdpath "path"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
)

func Mkdir(ctx context.Context, path string) (model.Obj, error) {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return nil, err
	}
	if !user.CanWrite() || !user.CanSMBManage() {
		meta, err := op.GetNearestMeta(stdpath.Dir(reqPath))
		if err != nil {
			if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
				return nil, err
			}
		}
		if !common.CanWrite(meta, reqPath) {
			return nil, errs.PermissionDenied
		}
	}
	if err = fs.MakeDir(ctx, reqPath); err != nil {
		return nil, err
	}
	return fs.Get(ctx, reqPath, &fs.GetArgs{})
}

func Remove(ctx context.Context, path string) error {
	user := ctx.Value("user").(*model.User)
	if !user.CanRemove() || !user.CanSMBManage() {
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
		if !user.CanRename() || !user.CanSMBManage() {
			return errs.PermissionDenied
		}
		return fs.Rename(ctx, srcPath, dstBase)
	} else {
		if !user.CanSMBManage() || !user.CanMove() || (srcBase != dstBase && !user.CanRename()) {
			return errs.PermissionDenied
		}
		if err = fs.Move(ctx, srcPath, dstDir); err != nil {
			return err
		}
		if srcBase != dstBase {
			return fs.Rename(ctx, stdpath.Join(dstDir, srcBase), dstBase)
		}
		return nil
	}
}
