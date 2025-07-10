package model

// Role represents a permission template which can be bound to users.
type Role struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"unique" binding:"required"`
	Description string `json:"description"`
	BasePaths   Paths  `json:"base_paths" gorm:"type:text"`
	// Determine permissions by bit, see User for details
	Permission int32 `json:"permission"`
}

func (r *Role) CheckPathLimit() bool {
	return (r.Permission>>14)&1 == 1
}
