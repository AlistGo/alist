package db

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
)

func GetSSHPublicKeyByUserId(userId uint, pageIndex, pageSize int) (keys []model.SSHPublicKey, count int64, err error) {
	keyDB := db.Model(&model.SSHPublicKey{})
	query := model.SSHPublicKey{UserId: userId}
	if err := keyDB.Where(query).Count(&count).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get user's keys count")
	}
	if err := keyDB.Where(query).Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&keys).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get find user's keys")
	}
	return keys, count, nil
}

func GetSSHPublicKeyById(id uint) (*model.SSHPublicKey, error) {
	var k model.SSHPublicKey
	if err := db.First(&k, id).Error; err != nil {
		return nil, errors.Wrapf(err, "failed get old key")
	}
	return &k, nil
}

func GetSSHPublicKeyByUserTitle(userId uint, title string) (*model.SSHPublicKey, error) {
	key := model.SSHPublicKey{UserId: userId, Title: title}
	if err := db.Where(key).First(&key).Error; err != nil {
		return nil, errors.Wrapf(err, "failed find key with title of user")
	}
	return &key, nil
}

func CreateSSHPublicKey(k *model.SSHPublicKey) error {
	return errors.WithStack(db.Create(k).Error)
}

func UpdateSSHPublicKey(k *model.SSHPublicKey) error {
	return errors.WithStack(db.Save(k).Error)
}

func GetSSHPublicKeys(pageIndex, pageSize int) (keys []model.SSHPublicKey, count int64, err error) {
	keyDB := db.Model(&model.SSHPublicKey{})
	if err := keyDB.Count(&count).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get keys count")
	}
	if err := keyDB.Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&keys).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get find keys")
	}
	return keys, count, nil
}

func DeleteSSHPublicKeyById(id uint) error {
	return errors.WithStack(db.Delete(&model.SSHPublicKey{}, id).Error)
}
