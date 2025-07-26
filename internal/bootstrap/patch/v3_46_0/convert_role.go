package v3_46_0

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/alist-org/alist/v3/internal/conf"
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

	rawDb := db.GetDb()
	table := conf.Conf.Database.TablePrefix + "users"
	rows, err := rawDb.Table(table).Select("id, username, role").Rows()
	if err != nil {
		utils.Log.Errorf("[convert roles] failed to get users: %v", err)
		return
	}
	defer rows.Close()

	var updatedCount int
	for rows.Next() {
		var id uint
		var username string
		var rawRole []byte

		if err := rows.Scan(&id, &username, &rawRole); err != nil {
			utils.Log.Warnf("[convert roles] skip user scan err: %v", err)
			continue
		}

		utils.Log.Debugf("[convert roles] user: %s raw role: %s", username, string(rawRole))

		if len(rawRole) == 0 {
			continue
		}

		var oldRoles []int
		wasSingleInt := false
		if err := json.Unmarshal(rawRole, &oldRoles); err != nil {
			var single int
			if err := json.Unmarshal(rawRole, &single); err != nil {
				utils.Log.Warnf("[convert roles] user %s has invalid role: %s", username, string(rawRole))
				continue
			}
			oldRoles = []int{single}
			wasSingleInt = true
		}

		var newRoles model.Roles
		for _, r := range oldRoles {
			switch r {
			case model.ADMIN:
				newRoles = append(newRoles, int(adminRole.ID))
			case model.GUEST:
				newRoles = append(newRoles, int(guestRole.ID))
			case model.GENERAL:
				newRoles = append(newRoles, int(generalRole.ID))
			default:
				newRoles = append(newRoles, r)
			}
		}

		if wasSingleInt {
			err := rawDb.Table(table).Where("id = ?", id).Update("role", newRoles).Error
			if err != nil {
				utils.Log.Errorf("[convert roles] failed to update user %s: %v", username, err)
			} else {
				updatedCount++
				utils.Log.Infof("[convert roles] updated user %s: %v → %v", username, oldRoles, newRoles)
			}
		}
	}

	utils.Log.Infof("[convert roles] completed role conversion for %d users", updatedCount)
}

func IsLegacyRoleDetected() bool {
	rawDb := db.GetDb()
	table := conf.Conf.Database.TablePrefix + "users"
	rows, err := rawDb.Table(table).Select("role").Rows()
	if err != nil {
		utils.Log.Errorf("[role check] failed to scan user roles: %v", err)
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var raw sql.RawBytes
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		if len(raw) == 0 {
			continue
		}

		var roles []int
		if err := json.Unmarshal(raw, &roles); err == nil {
			continue
		}

		var single int
		if err := json.Unmarshal(raw, &single); err == nil {
			utils.Log.Infof("[role check] detected legacy int role: %d", single)
			return true
		}
	}
	return false
}
