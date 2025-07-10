package data

import (
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func initRoles() {
	adminRole, err := op.GetRoleByName("admin")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			adminRole = &model.Role{
				Name:        "admin",
				Description: "Administrator",
				BasePath:    "/",
				Permission:  0x30FF,
			}
			if err := op.CreateRole(adminRole); err != nil {
				utils.Log.Fatalf("[init role] Failed to create admin role: %v", err)
			}
		} else {
			utils.Log.Fatalf("[init role] Failed to get admin role: %v", err)
		}
	}

	_, err = op.GetRoleByName("guest")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			guest := &model.Role{
				Name:        "guest",
				Description: "Guest",
				BasePath:    "/",
				Permission:  0,
			}
			if err := op.CreateRole(guest); err != nil {
				utils.Log.Fatalf("[init role] Failed to create guest role: %v", err)
			}
		} else {
			utils.Log.Fatalf("[init role] Failed to get guest role: %v", err)
		}
	}
}
