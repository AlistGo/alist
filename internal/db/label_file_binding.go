package db

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// GetLabelIds Get all label_ids from database order by file_name
func GetLabelIds(userId uint, fileName string) ([]uint, error) {
	//fmt.Printf(">>> [GetLabelIds] userId: %d, fileName: %s\n", userId, fileName)
	labelFileBinDingDB := db.Model(&model.LabelFileBinding{})
	var labelIds []uint
	if err := labelFileBinDingDB.Where("file_name = ?", fileName).Where("user_id = ?", userId).Pluck("label_id", &labelIds).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return labelIds, nil
}

func CreateLabelFileBinDing(fileName string, labelId, userId uint) error {
	var labelFileBinDing model.LabelFileBinding
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
	var labelFileBinDing model.LabelFileBinding
	result := db.Where("label_id = ?", labelId).Where("user_id = ?", userId).First(&labelFileBinDing)
	exists := !errors.Is(result.Error, gorm.ErrRecordNotFound)
	return exists
}

// DelLabelFileBinDingByFileName used to del usually
func DelLabelFileBinDingByFileName(userId uint, fileName string) error {
	return errors.WithStack(db.Where("file_name = ?", fileName).Where("user_id = ?", userId).Delete(model.LabelFileBinding{}).Error)
}

// DelLabelFileBinDingById used to del usually
func DelLabelFileBinDingById(labelId, userId uint, fileName string) error {
	return errors.WithStack(db.Where("label_id = ?", labelId).Where("file_name = ?", fileName).Where("user_id = ?", userId).Delete(model.LabelFileBinding{}).Error)
}

func GetLabelFileBinDingByLabelId(labelIds []uint, userId uint) (result []model.LabelFileBinding, err error) {
	if err := db.Where("label_id in (?)", labelIds).Where("user_id = ?", userId).Find(&result).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}

func GetLabelBindingsByFileNamesPublic(fileNames []string) (map[string][]uint, error) {
	var binds []model.LabelFileBinding
	if err := db.Where("file_name IN ?", fileNames).Find(&binds).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	out := make(map[string][]uint, len(fileNames))
	seen := make(map[string]struct{}, len(binds))
	for _, b := range binds {
		key := fmt.Sprintf("%s-%d", b.FileName, b.LabelId)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out[b.FileName] = append(out[b.FileName], b.LabelId)
	}
	return out, nil
}

func GetLabelsByFileNamesPublic(fileNames []string) (map[string][]model.Label, error) {
	bindMap, err := GetLabelBindingsByFileNamesPublic(fileNames)
	if err != nil {
		return nil, err
	}

	idSet := make(map[uint]struct{})
	for _, ids := range bindMap {
		for _, id := range ids {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return make(map[string][]model.Label, 0), nil
	}
	allIDs := make([]uint, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	labels, err := GetLabelByIds(allIDs) // 你已有的函数
	if err != nil {
		return nil, err
	}

	labelByID := make(map[uint]model.Label, len(labels))
	for _, l := range labels {
		labelByID[l.ID] = l
	}

	out := make(map[string][]model.Label, len(bindMap))
	for fname, ids := range bindMap {
		for _, id := range ids {
			if lab, ok := labelByID[id]; ok {
				out[fname] = append(out[fname], lab)
			}
		}
	}
	return out, nil
}

func ListLabelFileBinDing(userId uint, labelIDs []uint, fileName string, page, pageSize int) ([]model.LabelFileBinding, int64, error) {
	q := db.Model(&model.LabelFileBinding{}).Where("user_id = ?", userId)

	if len(labelIDs) > 0 {
		q = q.Where("label_id IN ?", labelIDs)
	}
	if fileName != "" {
		q = q.Where("file_name LIKE ?", "%"+fileName+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}

	var rows []model.LabelFileBinding
	if err := q.
		Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return rows, total, nil
}

func RestoreLabelFileBindings(bindings []model.LabelFileBinding, keepIDs bool, override bool) error {
	if len(bindings) == 0 {
		return nil
	}
	tx := db.Begin()

	if override {
		type key struct {
			uid  uint
			name string
		}
		toDel := make(map[key]struct{}, len(bindings))
		for i := range bindings {
			k := key{uid: bindings[i].UserId, name: bindings[i].FileName}
			toDel[k] = struct{}{}
		}
		for k := range toDel {
			if err := tx.Where("user_id = ? AND file_name = ?", k.uid, k.name).
				Delete(&model.LabelFileBinding{}).Error; err != nil {
				tx.Rollback()
				return errors.WithStack(err)
			}
		}
	}

	for i := range bindings {
		b := bindings[i]
		if !keepIDs {
			b.ID = 0
		}
		if b.CreateTime.IsZero() {
			b.CreateTime = time.Now()
		}
		if override {
			if err := tx.Create(&b).Error; err != nil {
				tx.Rollback()
				return errors.WithStack(err)
			}
		} else {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&b).Error; err != nil {
				tx.Rollback()
				return errors.WithStack(err)
			}
		}
	}

	return errors.WithStack(tx.Commit().Error)
}
