package db

import (
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetShareByShareID(shareID string) (*model.Share, error) {
	var share model.Share
	if err := db.Where("share_id = ?", shareID).Take(&share).Error; err != nil {
		return nil, err
	}
	return &share, nil
}

func GetShareByCreatorAndShareID(creatorID uint, shareID string) (*model.Share, error) {
	var share model.Share
	if err := db.Where("creator_id = ? AND share_id = ?", creatorID, shareID).Take(&share).Error; err != nil {
		return nil, err
	}
	return &share, nil
}

func ShareIDExists(shareID string) bool {
	var count int64
	if err := db.Model(&model.Share{}).Where("share_id = ?", shareID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func ShareIDExistsExceptID(shareID string, id uint) bool {
	var count int64
	if err := db.Model(&model.Share{}).Where("share_id = ? AND id <> ?", shareID, id).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func CreateShare(share *model.Share) error {
	return db.Create(share).Error
}

func UpdateShare(share *model.Share) error {
	return db.Save(share).Error
}

func GetSharesByCreator(creatorID uint, pageIndex, pageSize int) (shares []model.Share, count int64, err error) {
	tx := db.Model(&model.Share{}).Where("creator_id = ?", creatorID)
	err = tx.Count(&count).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("created_at desc").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&shares).Error
	return
}

func DeleteShareByShareID(creatorID uint, shareID string) error {
	return db.Where("creator_id = ? AND share_id = ?", creatorID, shareID).Delete(&model.Share{}).Error
}

func DisableShareByShareID(creatorID uint, shareID string) error {
	return db.Model(&model.Share{}).
		Where("creator_id = ? AND share_id = ?", creatorID, shareID).
		Update("enabled", false).Error
}

func TouchShareView(shareID string) error {
	now := time.Now()
	return db.Model(&model.Share{}).
		Where("share_id = ?", shareID).
		UpdateColumns(map[string]interface{}{
			"last_access_at": now,
			"view_count":     gorm.Expr("view_count + ?", 1),
		}).Error
}

func TouchShareDownload(shareID string) error {
	now := time.Now()
	return db.Model(&model.Share{}).
		Where("share_id = ?", shareID).
		UpdateColumns(map[string]interface{}{
			"last_access_at": now,
			"download_count": gorm.Expr("download_count + ?", 1),
		}).Error
}

func RecordShareAccess(shareID string) (*model.Share, error) {
	var updated model.Share
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("share_id = ?", shareID).
			Take(&updated).Error; err != nil {
			return err
		}

		now := time.Now()
		updated.AccessCount++
		updated.LastAccessAt = &now
		updates := map[string]interface{}{
			"access_count":   updated.AccessCount,
			"last_access_at": now,
		}

		limit := updated.EffectiveAccessLimit()
		if limit > 0 && updated.AccessCount >= limit {
			updated.Enabled = false
			updated.ConsumedAt = &now
			updates["enabled"] = false
			updates["consumed_at"] = now
		}

		return tx.Model(&model.Share{}).
			Where("id = ?", updated.ID).
			Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}
