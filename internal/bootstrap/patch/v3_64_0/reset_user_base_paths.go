package v3_64_0

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"gorm.io/gorm"
)

// ResetUserBasePaths transfers legacy directory restrictions to roles before
// removing the browsing prefix. All role and user writes commit together.
func ResetUserBasePaths() error {
	var updated int
	if err := db.GetDb().Transaction(func(tx *gorm.DB) error {
		var users []model.User
		if err := tx.Order("id").Find(&users).Error; err != nil {
			return err
		}
		var roles []model.Role
		if err := tx.Find(&roles).Error; err != nil {
			return err
		}
		originals := make(map[int]model.Role, len(roles))
		names := make(map[string]bool, len(roles))
		for _, r := range roles {
			originals[int(r.ID)] = r
			names[r.Name] = true
		}

		// GUEST is also an identity marker (anonymous login and account protection).
		// When a guest needs restriction, retain that marker but move its grants to
		// per-user roles. Root users sharing it reuse the original-grants template.
		splitGuest := false
		for _, u := range users {
			if !u.IsAdmin() && u.IsGuest() && utils.FixAndCleanPath(u.BasePath) != "/" {
				splitGuest = true
			}
		}
		var guestTemplateID int
		if splitGuest {
			// Preserve the guest template for future registrations as well as
			// existing users. The original role ID remains an identity marker.
			original := originals[model.GUEST]
			name := "migrated-guest-permissions"
			for suffix := 1; names[name]; suffix++ {
				name = fmt.Sprintf("migrated-guest-permissions-%d", suffix)
			}
			template := model.Role{Name: name, Description: "Guest permissions before base path migration", Default: original.Default, PermissionScopes: original.PermissionScopes}
			if err := tx.Create(&template).Error; err != nil {
				return err
			}
			guestTemplateID = int(template.ID)
			if err := tx.Model(&model.SettingItem{}).Where(map[string]interface{}{"key": conf.DefaultRole, "value": []string{strconv.Itoa(model.GUEST), original.Name}}).UpdateColumn("value", strconv.Itoa(int(template.ID))).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Role{}).Where("id = ?", model.GUEST).UpdateColumn("default", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Role{}).Where("id = ?", model.GUEST).UpdateColumn("raw_permission", "").Error; err != nil {
				return err
			}
		}
		for _, u := range users {
			base := utils.FixAndCleanPath(u.BasePath)
			migrate := !u.IsAdmin() && base != "/"
			bindings := u.Role
			if migrate {
				scopes := make(map[string]int32)
				for _, rid := range u.Role {
					role, ok := originals[rid]
					if !ok {
						return fmt.Errorf("user %d references missing role %d", u.ID, rid)
					}
					for _, entry := range role.PermissionScopes {
						scope := utils.FixAndCleanPath(entry.Path)
						switch {
						case utils.IsSubPath(scope, base):
							scope = base
						case utils.IsSubPath(base, scope):
						default:
							continue
						}
						scopes[scope] |= entry.Permission
					}
				}
				entries := make([]model.PermissionEntry, 0, len(scopes))
				for p, perm := range scopes {
					entries = append(entries, model.PermissionEntry{Path: p, Permission: perm})
				}
				sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
				name := fmt.Sprintf("migrated-base-path-user-%d", u.ID)
				for suffix := 1; names[name]; suffix++ {
					name = fmt.Sprintf("migrated-base-path-user-%d-%d", u.ID, suffix)
				}
				role := model.Role{Name: name, Description: fmt.Sprintf("Migrated user %d base path %s", u.ID, base), PermissionScopes: entries}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
				names[name] = true
				bindings = model.Roles{int(role.ID)}
				if u.IsGuest() {
					bindings = append(model.Roles{model.GUEST}, bindings...)
				}
			}
			// Root users need no per-user role. If their shared guest grants were
			// split, reuse the single template created for that split.
			reuseGuestTemplate := splitGuest && u.IsGuest() && !migrate
			if reuseGuestTemplate {
				bindings = append(append(model.Roles(nil), u.Role...), guestTemplateID)
			}
			if u.BasePath == "/" && !migrate && !reuseGuestTemplate {
				continue
			}
			if err := tx.Model(&model.User{}).Where("id = ?", u.ID).
				UpdateColumns(map[string]interface{}{"base_path": "/", "role": bindings}).Error; err != nil {
				return err
			}
			updated++
		}
		return nil
	}); err != nil {
		return fmt.Errorf("migrate user base paths to roles: %w", err)
	}
	// No transaction-local objects may escape into application caches.
	op.ClearUserCache()
	op.ClearRoleCache()
	op.SettingCacheUpdate()
	utils.Log.Infof("[reset user base paths] migrated %d users to /", updated)
	return nil
}
