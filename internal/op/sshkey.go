package op

import (
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"time"
)

func CreateSSHPublicKey(k *model.SSHPublicKey) (error, bool) {
	_, err := db.GetSSHPublicKeyByUserTitle(k.UserId, k.Title)
	if err == nil {
		return errors.New("key with the same title already exists"), true
	}
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.KeyStr))
	if err != nil {
		return err, false
	}
	k.Fingerprint = ssh.FingerprintSHA256(pubKey)
	k.AddedTime = time.Now()
	k.LastUsedTime = k.AddedTime
	return db.CreateSSHPublicKey(k), true
}

func GetSSHPublicKeyByUserId(userId uint, pageIndex, pageSize int) (keys []model.SSHPublicKey, count int64, err error) {
	return db.GetSSHPublicKeyByUserId(userId, pageIndex, pageSize)
}

func GetSSHPublicKeyByIdAndUserId(id uint, userId uint) (*model.SSHPublicKey, error) {
	key, err := db.GetSSHPublicKeyById(id)
	if err != nil {
		return nil, err
	}
	if key.UserId != userId {
		return nil, errors.Wrapf(err, "failed get old key")
	}
	return key, nil
}

func UpdateSSHPublicKey(k *model.SSHPublicKey) error {
	return db.UpdateSSHPublicKey(k)
}

func DeleteSSHPublicKeyById(keyId uint) error {
	return db.DeleteSSHPublicKeyById(keyId)
}
