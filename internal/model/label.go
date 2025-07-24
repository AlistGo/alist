package model

import "time"

type Label struct {
	ID          uint      `json:"id" gorm:"primaryKey"` // unique key
	Type        int       `json:"type"`                 // use to type
	Name        string    `json:"name"`                 // use to name
	Description string    `json:"description"`          // use to description
	BgColor     string    `json:"bg_color"`             // use to bg_color
	CreateTime  time.Time `json:"create_time"`
}
