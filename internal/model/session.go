package model

// Session represents a device session of a user.
type Session struct {
	UserID     uint   `json:"user_id" gorm:"index"`
	DeviceKey  string `json:"device_key" gorm:"primaryKey;size:64"`
	UserAgent  string `json:"user_agent" gorm:"size:255"`
	IP         string `json:"ip" gorm:"size:64"`
	LastActive int64  `json:"last_active"`
	Status     int    `json:"status"`
}

const (
	SessionActive = iota
	SessionInactive
)
