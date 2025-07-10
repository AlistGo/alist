package model

// Role represents a permission template which can be bound to users.
type Role struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"unique" binding:"required"`
	Description string `json:"description"`
	BasePath    string `json:"base_path"`
	Permission  int32  `json:"permission"`
}
