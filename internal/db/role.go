package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"path"
	"strings"
)

func GetRole(id uint) (*model.Role, error) {
	var r model.Role
	if err := db.First(&r, id).Error; err != nil {
		return nil, errors.Wrapf(err, "failed get role")
	}
	return &r, nil
}

func GetRoleByName(name string) (*model.Role, error) {
	r := model.Role{Name: name}
	if err := db.Where(r).First(&r).Error; err != nil {
		return nil, errors.Wrapf(err, "failed get role")
	}
	return &r, nil
}

func GetRoles(pageIndex, pageSize int) (roles []model.Role, count int64, err error) {
	roleDB := db.Model(&model.Role{})
	if err = roleDB.Count(&count).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get roles count")
	}
	if err = roleDB.Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&roles).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get find roles")
	}
	return roles, count, nil
}

func GetAllRoles() ([]model.Role, error) {
	var roles []model.Role
	if err := db.Find(&roles).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return roles, nil
}

func CreateRole(r *model.Role) error {
	if err := db.Create(r).Error; err != nil {
		return errors.WithStack(err)
	}
	if r.Default {
		if err := db.Model(&model.Role{}).Where("id <> ?", r.ID).Update("default", false).Error; err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func UpdateRole(r *model.Role) error {
	if err := db.Save(r).Error; err != nil {
		return errors.WithStack(err)
	}
	if r.Default {
		if err := db.Model(&model.Role{}).Where("id <> ?", r.ID).Update("default", false).Error; err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func DeleteRole(id uint) error {
	return errors.WithStack(db.Delete(&model.Role{}, id).Error)
}

func UpdateRolePermissionsPathPrefix(oldPath, newPath string) ([]uint, error) {
	var roles []model.Role
	var modifiedRoleIDs []uint

	if err := db.Find(&roles).Error; err != nil {
		return nil, errors.WithMessage(err, "failed to load roles")
	}

	for _, role := range roles {
		updated := false
		for i, entry := range role.PermissionScopes {
			entryPath := path.Clean(entry.Path)
			oldPathClean := path.Clean(oldPath)

			if entryPath == oldPathClean {
				role.PermissionScopes[i].Path = newPath
				updated = true
			} else if strings.HasPrefix(entryPath, oldPathClean+"/") {
				role.PermissionScopes[i].Path = newPath + entryPath[len(oldPathClean):]
				updated = true
			}
		}
		if updated {
			if err := UpdateRole(&role); err != nil {
				return nil, errors.WithMessagef(err, "failed to update role ID %d", role.ID)
			}
			modifiedRoleIDs = append(modifiedRoleIDs, role.ID)
		}
	}
	return modifiedRoleIDs, nil
}
