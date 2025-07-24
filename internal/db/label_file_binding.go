package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"time"
)

// GetLabelIds Get all label_ids from database order by file_name
func GetLabelIds(userId uint, fileName string) ([]uint, error) {
	labelFileBinDingDB := db.Model(&model.LabelFileBinDing{})
	var labelIds []uint
	if err := labelFileBinDingDB.Where("file_name = ?", fileName).Where("user_id = ?", userId).Pluck("label_id", &labelIds).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return labelIds, nil
}

func CreateLabelFileBinDing(fileName string, labelId, userId uint) error {
	var labelFileBinDing model.LabelFileBinDing
	labelFileBinDing.UserId = userId
	labelFileBinDing.LabelId = labelId
	labelFileBinDing.FileName = fileName
	labelFileBinDing.CreateTime = time.Now()
	err := errors.WithStack(db.Create(&labelFileBinDing).Error)
	if err != nil {
		return errors.WithMessage(err, "failed create label in database")
	}
	return nil
}

// GetLabelFileBinDingByLabelIdExists Get Label by label_id, used to del label usually
func GetLabelFileBinDingByLabelIdExists(labelId, userId uint) bool {
	var labelFileBinDing model.LabelFileBinDing
	result := db.Where("label_id = ?", labelId).Where("user_id = ?", userId).First(&labelFileBinDing)
	exists := !errors.Is(result.Error, gorm.ErrRecordNotFound)
	return exists
}

// DelLabelFileBinDingByFileName used to del usually
func DelLabelFileBinDingByFileName(userId uint, fileName string) error {
	return errors.WithStack(db.Where("file_name = ?", fileName).Where("user_id = ?", userId).Delete(model.LabelFileBinDing{}).Error)
}

// DelLabelFileBinDingById used to del usually
func DelLabelFileBinDingById(labelId, userId uint, fileName string) error {
	return errors.WithStack(db.Where("label_id = ?", labelId).Where("file_name = ?", fileName).Where("user_id = ?", userId).Delete(model.LabelFileBinDing{}).Error)
}

func GetLabelFileBinDingByLabelId(labelIds []uint, userId uint) (result []model.LabelFileBinDing, err error) {
	if err := db.Where("label_id in (?)", labelIds).Where("user_id = ?", userId).Find(&result).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}
