package fs

import (
	"context"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// List files
func list(ctx context.Context, path string, args *ListArgs) ([]model.Obj, error) {
	meta, _ := ctx.Value("meta").(*model.Meta)
	user, _ := ctx.Value("user").(*model.User)
	virtualFiles := op.GetStorageVirtualFilesByPath(path)
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil && len(virtualFiles) == 0 {
		return nil, errors.WithMessage(err, "failed get storage")
	}

	var _objs []model.Obj
	if storage != nil {
		_objs, err = op.List(ctx, storage, actualPath, model.ListArgs{
			ReqPath: path,
			Refresh: args.Refresh,
		})
		if err != nil {
			if !args.NoLog {
				log.Errorf("fs/list: %+v", err)
			}
			if len(virtualFiles) == 0 {
				return nil, errors.WithMessage(err, "failed get objs")
			}
		}
	}

	om := model.NewObjMerge()
	if whetherHide(user, meta, path) {
		om.InitHideReg(meta.Hide)
	}
	objs := om.Merge(_objs, virtualFiles...)
	return objs, nil
}

func whetherHide(user *model.User, meta *model.Meta, path string) bool {
	// if user is nil, don't hide
	if user == nil {
		return false
	}
	perm := common.MergeRolePermissions(user, path)
	// if user has see-hides permission, don't hide
	if common.HasPermission(perm, common.PermSeeHides) {
		return false
	}
	// if meta is nil, don't hide
	if meta == nil {
		return false
	}
	// if meta.Hide is empty, don't hide
	if meta.Hide == "" {
		return false
	}
	// if meta doesn't apply to sub_folder, don't hide
	if !utils.PathEqual(meta.Path, path) && !meta.HSub {
		return false
	}
	// if is guest, hide
	return true
}
