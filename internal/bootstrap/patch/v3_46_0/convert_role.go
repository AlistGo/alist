package v3_46_0

import (
	"errors"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"gorm.io/gorm"
)

// ConvertLegacyRoles migrates old integer role values to a new role model with permission scopes.
func ConvertLegacyRoles() {
	guestRole, err := op.GetRoleByName("guest")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			guestRole = &model.Role{
				ID:          uint(model.GUEST),
				Name:        "guest",
				Description: "Guest",
				PermissionScopes: []model.PermissionEntry{
					{
						Path:       "/",
						Permission: 0,
					},
				},
			}
			if err = op.CreateRole(guestRole); err != nil {
				utils.Log.Errorf("[convert roles] failed to create guest role: %v", err)
				return
			}
		} else {
			utils.Log.Errorf("[convert roles] failed to get guest role: %v", err)
			return
		}
	}

	adminRole, err := op.GetRoleByName("admin")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			adminRole = &model.Role{
				ID:          uint(model.ADMIN),
				Name:        "admin",
				Description: "Administrator",
				PermissionScopes: []model.PermissionEntry{
					{
						Path:       "/",
						Permission: 0x33FF,
					},
				},
			}
			if err = op.CreateRole(adminRole); err != nil {
				utils.Log.Errorf("[convert roles] failed to create admin role: %v", err)
				return
			}
		} else {
			utils.Log.Errorf("[convert roles] failed to get admin role: %v", err)
			return
		}
	}

	generalRole, err := op.GetRoleByName("general")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			generalRole = &model.Role{
				ID:          uint(model.NEWGENERAL),
				Name:        "general",
				Description: "General User",
				PermissionScopes: []model.PermissionEntry{
					{
						Path:       "/",
						Permission: 0,
					},
				},
			}
			if err = op.CreateRole(generalRole); err != nil {
				utils.Log.Errorf("[convert roles] failed create general role: %v", err)
				return
			}
		} else {
			utils.Log.Errorf("[convert roles] failed get general role: %v", err)
			return
		}
	}

	users, _, err := op.GetUsers(1, -1)
	if err != nil {
		utils.Log.Errorf("[convert roles] failed to get users: %v", err)
		return
	}

	for i := range users {
		user := users[i]
		if user.Role == nil {
			continue
		}
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
				roles = append(roles, int(generalRole.ID))
				if int(generalRole.ID) != r {
					changed = true
				}
			default:
				roles = append(roles, r)
			}
		}
		if changed {
			user.Role = roles
			if err := db.UpdateUser(&user); err != nil {
				utils.Log.Errorf("[convert roles] failed to update user %s: %v", user.Username, err)
			}
		}
	}

	utils.Log.Infof("[convert roles] completed role conversion for %d users", len(users))
}
