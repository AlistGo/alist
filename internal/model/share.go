package model

import "time"

type Share struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	ShareID       string     `json:"share_id" gorm:"uniqueIndex;size:32;not null"`
	CreatorID     uint       `json:"creator_id" gorm:"index;not null"`
	Name          string     `json:"name" gorm:"size:255;not null"`
	RootPath      string     `json:"root_path" gorm:"size:4096;not null"`
	IsDir         bool       `json:"is_dir"`
	PasswordHash  string     `json:"-" gorm:"size:64"`
	PasswordSalt  string     `json:"-" gorm:"size:32"`
	BurnAfterRead bool       `json:"burn_after_read" gorm:"default:false"`
	AccessLimit   int64      `json:"access_limit"`
	AccessCount   int64      `json:"access_count"`
	AllowPreview  bool       `json:"allow_preview" gorm:"default:true"`
	AllowDownload bool       `json:"allow_download" gorm:"default:true"`
	Enabled       bool       `json:"enabled" gorm:"default:true;index"`
	ViewCount     int64      `json:"view_count"`
	DownloadCount int64      `json:"download_count"`
	LastAccessAt  *time.Time `json:"last_access_at"`
	ConsumedAt    *time.Time `json:"consumed_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (s Share) HasPassword() bool {
	return s.PasswordHash != ""
}

func (s Share) EffectiveAccessLimit() int64 {
	if s.AccessLimit > 0 {
		return s.AccessLimit
	}
	if s.BurnAfterRead {
		return 1
	}
	return 0
}

func (s Share) RemainingAccesses() int64 {
	limit := s.EffectiveAccessLimit()
	if limit <= 0 {
		return 0
	}
	remaining := limit - s.AccessCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s Share) IsConsumed() bool {
	limit := s.EffectiveAccessLimit()
	return s.ConsumedAt != nil || (limit > 0 && s.AccessCount >= limit)
}

func (s Share) IsExpired(now time.Time) bool {
	return s.ExpiresAt != nil && !s.ExpiresAt.After(now)
}
