package model

import (
	"encoding/json"

	"gorm.io/gorm"
)

// PermissionEntry defines permission bitmask for a specific path.
type PermissionEntry struct {
	Path       string `json:"path"`       // path prefix, e.g. "/admin"
	Permission int32  `json:"permission"` // bitmask permissions
}

// Role represents a permission template which can be bound to users.
type Role struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"unique" binding:"required"`
	Description string `json:"description"`
	Default     bool   `json:"default" gorm:"default:false"`
	// PermissionScopes stores structured permission list and is ignored by gorm.
	PermissionScopes []PermissionEntry `json:"permission_scopes" gorm:"-"`
	// RawPermission is the JSON representation of PermissionScopes stored in DB.
	RawPermission string `json:"-" gorm:"type:text"`
}

// BeforeSave GORM hook serializes PermissionScopes into RawPermission.
func (r *Role) BeforeSave(tx *gorm.DB) error {
	if len(r.PermissionScopes) == 0 {
		r.RawPermission = ""
		return nil
	}
	bs, err := json.Marshal(r.PermissionScopes)
	if err != nil {
		return err
	}
	r.RawPermission = string(bs)
	return nil
}

// AfterFind GORM hook deserializes RawPermission into PermissionScopes.
func (r *Role) AfterFind(tx *gorm.DB) error {
	if r.RawPermission == "" {
		r.PermissionScopes = nil
		return nil
	}
	var scopes []PermissionEntry
	if err := json.Unmarshal([]byte(r.RawPermission), &scopes); err != nil {
		return err
	}
	r.PermissionScopes = scopes
	return nil
}
