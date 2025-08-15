// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webdav

import (
	"context"
	"net/http"
	"path"
	"path/filepath"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
)

// slashClean is equivalent to but slightly more efficient than
// path.Clean("/" + name).
func slashClean(name string) string {
	if name == "" || name[0] != '/' {
		name = "/" + name
	}
	return path.Clean(name)
}

// moveFiles moves files and/or directories from src to dst.
//
// See section 9.9.4 for when various HTTP status codes apply.
func moveFiles(ctx context.Context, src, dst string, overwrite bool) (status int, err error) {
	srcDir := path.Dir(src)
	dstDir := path.Dir(dst)
	srcName := path.Base(src)
	dstName := path.Base(dst)
	user := ctx.Value("user").(*model.User)
	perm := common.MergeRolePermissions(user, src)
	if srcDir != dstDir && !common.HasPermission(perm, common.PermMove) {
		return http.StatusForbidden, nil
	}
	if srcName != dstName && !common.HasPermission(perm, common.PermRename) {
		return http.StatusForbidden, nil
	}
	if srcDir == dstDir {
		err = fs.Rename(ctx, src, dstName)
	} else {
		err = fs.Move(ctx, src, dstDir)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if srcName != dstName {
			err = fs.Rename(ctx, path.Join(dstDir, srcName), dstName)
		}
	}
	if err != nil {
		return http.StatusInternalServerError, err
	}
	// TODO if there are no files copy, should return 204
	return http.StatusCreated, nil
}

// copyFiles copies files and/or directories from src to dst.
//
// See section 9.8.5 for when various HTTP status codes apply.
func copyFiles(ctx context.Context, src, dst string, overwrite bool) (status int, err error) {
	dstDir := path.Dir(dst)
	_, err = fs.Copy(context.WithValue(ctx, conf.NoTaskKey, struct{}{}), src, dstDir)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	// TODO if there are no files copy, should return 204
	return http.StatusCreated, nil
}

// walkFS traverses filesystem fs starting at name up to depth levels.
//
// Allowed values for depth are 0, 1 or infiniteDepth. For each visited node,
// walkFS calls walkFn. If a visited file system node is a directory and
// walkFn returns path.SkipDir, walkFS will skip traversal of this node.
func walkFS(ctx context.Context, depth int, name string, info model.Obj, walkFn func(reqPath string, info model.Obj, err error) error) error {
	// This implementation is based on Walk's code in the standard path/path package.
	err := walkFn(name, info, nil)
	if err != nil {
		if info.IsDir() && err == filepath.SkipDir {
			return nil
		}
		return err
	}
	if !info.IsDir() || depth == 0 {
		return nil
	}
	if depth == 1 {
		depth = 0
	}
	meta, _ := op.GetNearestMeta(name)
	user := ctx.Value("user").(*model.User)
	// Read directory names.
	objs, err := fs.List(context.WithValue(ctx, "meta", meta), name, &fs.ListArgs{})
	//f, err := fs.OpenFile(ctx, name, os.O_RDONLY, 0)
	//if err != nil {
	//	return walkFn(name, info, err)
	//}
	//fileInfos, err := f.Readdir(0)
	//f.Close()
	if err != nil {
		return walkFn(name, info, err)
	}

	for _, fileInfo := range objs {
		filename := path.Join(name, fileInfo.GetName())
		if !common.CanReadPathByRole(user, filename) {
			continue
		}
		if err != nil {
			if err := walkFn(filename, fileInfo, err); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = walkFS(ctx, depth, filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err
				}
			}
		}
	}
	return nil
}
