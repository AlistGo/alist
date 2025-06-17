package model

import (
	"golang.org/x/crypto/ssh"
	"time"
)

type SSHPublicKey struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	UserId       uint      `json:"-"`
	Title        string    `json:"title"`
	Fingerprint  string    `json:"fingerprint"`
	KeyStr       string    `gorm:"type:text" json:"-"`
	AddedTime    time.Time `json:"added_time"`
	LastUsedTime time.Time `json:"last_used_time"`
}

func (k *SSHPublicKey) GetKey() (ssh.PublicKey, error) {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k.KeyStr))
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}

func (k *SSHPublicKey) UpdateLastUsedTime() {
	k.LastUsedTime = time.Now()
}
