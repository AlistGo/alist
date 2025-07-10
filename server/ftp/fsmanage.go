package ftp

import (
	"context"
	"fmt"
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
	perm := common.MergeRolePermissions(user, reqPath)
	if !common.HasPermission(perm, common.PermWrite) || !common.HasPermission(perm, common.PermFTPManage) {
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
	perm := common.MergeRolePermissions(user, path)
	if !common.HasPermission(perm, common.PermRemove) || !common.HasPermission(perm, common.PermFTPManage) {
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
	permSrc := common.MergeRolePermissions(user, srcPath)
	if srcDir == dstDir {
		if !common.HasPermission(permSrc, common.PermRename) || !common.HasPermission(permSrc, common.PermFTPManage) {
			return errs.PermissionDenied
		}
		return fs.Rename(ctx, srcPath, dstBase)
	} else {
		if !common.HasPermission(permSrc, common.PermFTPManage) || !common.HasPermission(permSrc, common.PermMove) || (srcBase != dstBase && !common.HasPermission(permSrc, common.PermRename)) {
			return errs.PermissionDenied
		}
		if err = fs.Move(ctx, srcPath, dstDir); err != nil {
			if srcBase != dstBase {
				return err
			}
			if _, err1 := fs.Copy(ctx, srcPath, dstDir); err1 != nil {
				return fmt.Errorf("failed move for %+v, and failed try copying for %+v", err, err1)
			}
			return nil
		}
		if srcBase != dstBase {
			return fs.Rename(ctx, stdpath.Join(dstDir, srcBase), dstBase)
		}
		return nil
	}
}
