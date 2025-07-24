package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// GetFileByNameExists Get file by name
func GetFileByNameExists(name string) bool {
	var label model.ObjFile
	result := db.Where("name = ?", name).First(&label)
	exists := !errors.Is(result.Error, gorm.ErrRecordNotFound)
	return exists
}

// GetFileByName Get file by name
func GetFileByName(name string, userId uint) (objFile model.ObjFile, err error) {
	if err = db.Where("name = ?", name).Where("user_id = ?", userId).First(&objFile).Error; err != nil {
		return objFile, errors.WithStack(err)
	}
	return objFile, nil
}

func CreateObjFile(obj model.ObjFile) error {
	err := errors.WithStack(db.Create(&obj).Error)
	if err != nil {
		return errors.WithMessage(err, "failed create file in database")
	}
	return nil
}
