package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"time"
)

// GetLabels Get all label from database order by id
func GetLabels(pageIndex, pageSize int) ([]model.Label, int64, error) {
	labelDB := db.Model(&model.Label{})
	var count int64
	if err := labelDB.Count(&count).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get label count")
	}
	var labels []model.Label
	if err := labelDB.Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&labels).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return labels, count, nil
}

// GetLabelById Get Label by id, used to update label usually
func GetLabelById(id uint) (*model.Label, error) {
	var label model.Label
	label.ID = id
	if err := db.First(&label).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return &label, nil
}

// CreateLabel just insert label to database
func CreateLabel(label model.Label) (uint, error) {
	label.CreateTime = time.Now()
	err := errors.WithStack(db.Create(&label).Error)
	if err != nil {
		return label.ID, errors.WithMessage(err, "failed create label in database")
	}
	return label.ID, nil
}

// UpdateLabel just update storage in database
func UpdateLabel(label *model.Label) (*model.Label, error) {
	label.CreateTime = time.Now()
	_, err := GetLabelById(label.ID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get old label")
	}
	err = errors.WithStack(db.Save(label).Error)
	if err != nil {
		return nil, errors.WithMessage(err, "failed create label in database")
	}
	return label, nil
}

// DeleteLabelById just delete label from database by id
func DeleteLabelById(id uint) error {
	return errors.WithStack(db.Delete(&model.Label{}, id).Error)
}

// GetLabelByIds Get label from database order by ids
func GetLabelByIds(ids []uint) ([]model.Label, error) {
	labelDB := db.Model(&model.Label{})
	var labels []model.Label
	if err := labelDB.Where(ids).Find(&labels).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return labels, nil
}

// GetLabelByName Get Label by name
func GetLabelByName(name string) bool {
	var label model.Label
	result := db.Where("name = ?", name).First(&label)
	exists := !errors.Is(result.Error, gorm.ErrRecordNotFound)
	return exists
}
