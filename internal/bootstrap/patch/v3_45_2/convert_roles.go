package v3_45_2

import (
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// ConvertLegacyRoles migrates old integer role values to role IDs
// and ensures default roles exist in the roles table.
func ConvertLegacyRoles() {
	// ensure the roles table exists
	if err := db.AutoMigrate(new(model.Role)); err != nil {
		utils.Log.Errorf("[convert roles] failed to auto migrate: %v", err)
		return
	}

	// create or get guest role
	guestRole, err := op.GetRoleByName("guest")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			guestRole = &model.Role{
				ID:          uint(model.GUEST),
				Name:        "guest",
				Description: "Guest",
				BasePaths:   model.Paths{"/"},
				Permission:  0,
			}
			if err = op.CreateRole(guestRole); err != nil {
				utils.Log.Errorf("[convert roles] failed create guest role: %v", err)
				return
			}
		} else {
			utils.Log.Errorf("[convert roles] failed get guest role: %v", err)
			return
		}
	}

	// create or get admin role
	adminRole, err := op.GetRoleByName("admin")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			adminRole = &model.Role{
				ID:          uint(model.ADMIN),
				Name:        "admin",
				Description: "Administrator",
				BasePaths:   model.Paths{"/"},
				Permission:  0x30FF,
			}
			if err = op.CreateRole(adminRole); err != nil {
				utils.Log.Errorf("[convert roles] failed create admin role: %v", err)
				return
			}
		} else {
			utils.Log.Errorf("[convert roles] failed get admin role: %v", err)
			return
		}
	}

	users, _, err := op.GetUsers(1, -1)
	if err != nil {
		utils.Log.Errorf("[convert roles] failed get users: %v", err)
		return
	}

	for i := range users {
		user := users[i]
		changed := false
		var roles model.Roles
		for _, r := range user.Role {
			switch r {
			case model.ADMIN:
				roles = append(roles, int(adminRole.ID))
				if int(adminRole.ID) != r {
					changed = true
				}
			case model.GUEST:
				roles = append(roles, int(guestRole.ID))
				if int(guestRole.ID) != r {
					changed = true
				}
			case model.GENERAL:
				changed = true
				// drop GENERAL role
			default:
				roles = append(roles, r)
			}
		}
		if changed {
			user.Role = roles
			if err := db.UpdateUser(&user); err != nil {
				utils.Log.Errorf("[convert roles] failed update user %s: %v", user.Username, err)
			}
		}
	}
}
