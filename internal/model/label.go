package model

import "time"

type Label struct {
	ID         uint      `json:"id" gorm:"primaryKey"` // unique key
	Type       int       `json:"type"`                 // use to type
	Name       string    `json:"name"`                 // use to name
	CreateTime time.Time `json:"create_time"`
}
