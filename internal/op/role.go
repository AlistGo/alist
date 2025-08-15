package op

import (
	"fmt"
	"time"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/singleflight"
	"github.com/alist-org/alist/v3/pkg/utils"
)

var roleCache = cache.NewMemCache[*model.Role](cache.WithShards[*model.Role](2))
var roleG singleflight.Group[*model.Role]

func init() {
	model.FetchRole = GetRole
}

func GetRole(id uint) (*model.Role, error) {
	key := fmt.Sprint(id)
	if r, ok := roleCache.Get(key); ok {
		return r, nil
	}
	r, err, _ := roleG.Do(key, func() (*model.Role, error) {
		_r, err := db.GetRole(id)
		if err != nil {
			return nil, err
		}
		roleCache.Set(key, _r, cache.WithEx[*model.Role](time.Hour))
		return _r, nil
	})
	return r, err
}

func GetRoleByName(name string) (*model.Role, error) {
	if r, ok := roleCache.Get(name); ok {
		return r, nil
	}
	r, err, _ := roleG.Do(name, func() (*model.Role, error) {
		_r, err := db.GetRoleByName(name)
		if err != nil {
			return nil, err
		}
		roleCache.Set(name, _r, cache.WithEx[*model.Role](time.Hour))
		return _r, nil
	})
	return r, err
}

func GetRolesByUserID(userID uint) ([]model.Role, error) {
	user, err := GetUserById(userID)
	if err != nil {
		return nil, err
	}

	var roles []model.Role
	for _, roleID := range user.Role {
		key := fmt.Sprint(roleID)

		if r, ok := roleCache.Get(key); ok {
			roles = append(roles, *r)
			continue
		}

		r, err, _ := roleG.Do(key, func() (*model.Role, error) {
			_r, err := db.GetRole(uint(roleID))
			if err != nil {
				return nil, err
			}
			roleCache.Set(key, _r, cache.WithEx[*model.Role](time.Hour))
			return _r, nil
		})
		if err != nil {
			return nil, err
		}
		roles = append(roles, *r)
	}

	return roles, nil
}

func GetRoles(pageIndex, pageSize int) ([]model.Role, int64, error) {
	return db.GetRoles(pageIndex, pageSize)
}

func CreateRole(r *model.Role) error {
	for i := range r.PermissionScopes {
		r.PermissionScopes[i].Path = utils.FixAndCleanPath(r.PermissionScopes[i].Path)
	}
	roleCache.Del(fmt.Sprint(r.ID))
	roleCache.Del(r.Name)
	return db.CreateRole(r)
}

func UpdateRole(r *model.Role) error {
	old, err := db.GetRole(r.ID)
	if err != nil {
		return err
	}
	switch old.Name {
	case "admin":
		return errs.ErrChangeDefaultRole
	case "guest":
		r.Name = "guest"
	}
	for i := range r.PermissionScopes {
		r.PermissionScopes[i].Path = utils.FixAndCleanPath(r.PermissionScopes[i].Path)
	}
	//if len(old.PermissionScopes) > 0 && len(r.PermissionScopes) > 0 &&
	//	old.PermissionScopes[0].Path != r.PermissionScopes[0].Path {
	//
	//	oldPath := old.PermissionScopes[0].Path
	//	newPath := r.PermissionScopes[0].Path
	//
	//	users, err := db.GetUsersByRole(int(r.ID))
	//	if err != nil {
	//		return errors.WithMessage(err, "failed to get users by role")
	//	}
	//
	//	modifiedUsernames, err := db.UpdateUserBasePathPrefix(oldPath, newPath, users)
	//	if err != nil {
	//		return errors.WithMessage(err, "failed to update user base path when role updated")
	//	}
	//
	//	for _, name := range modifiedUsernames {
	//		userCache.Del(name)
	//	}
	//}
	roleCache.Del(fmt.Sprint(r.ID))
	roleCache.Del(r.Name)
	return db.UpdateRole(r)
}

func DeleteRole(id uint) error {
	old, err := db.GetRole(id)
	if err != nil {
		return err
	}
	if old.Name == "admin" || old.Name == "guest" {
		return errs.ErrChangeDefaultRole
	}
	roleCache.Del(fmt.Sprint(id))
	roleCache.Del(old.Name)
	return db.DeleteRole(id)
}
