package model

import "time"

type LabelFileBinding struct {
	ID         uint      `json:"id" gorm:"primaryKey"` // unique key
	UserId     uint      `json:"user_id"`              // use to user_id
	LabelId    uint      `json:"label_id"`             // use to label_id
	FileName   string    `json:"file_name"`            // use to file_name
	CreateTime time.Time `json:"create_time"`
}
