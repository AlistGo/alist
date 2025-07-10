package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
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

func CreateRole(r *model.Role) error {
	return errors.WithStack(db.Create(r).Error)
}

func UpdateRole(r *model.Role) error {
	return errors.WithStack(db.Save(r).Error)
}

func DeleteRole(id uint) error {
	return errors.WithStack(db.Delete(&model.Role{}, id).Error)
}
