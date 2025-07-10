package data

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func initRoles() {
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
			if err := op.CreateRole(guestRole); err != nil {
				utils.Log.Fatalf("[init role] Failed to create guest role: %v", err)
			}
		} else {
			utils.Log.Fatalf("[init role] Failed to get guest role: %v", err)
		}
	}

	_, err = op.GetRoleByName("admin")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			adminRole := &model.Role{
				ID:          uint(model.ADMIN),
				Name:        "admin",
				Description: "Administrator",
				BasePaths:   model.Paths{"/"},
				Permission:  0x30FF,
			}
			if err := op.CreateRole(adminRole); err != nil {
				utils.Log.Fatalf("[init role] Failed to create admin role: %v", err)
			}
		} else {
			utils.Log.Fatalf("[init role] Failed to get admin role: %v", err)
		}
	}
}
