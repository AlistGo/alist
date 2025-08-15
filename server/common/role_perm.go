package common

import (
	"path"
	"strings"

	"github.com/dlclark/regexp2"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
)

const (
	PermSeeHides = iota
	PermAccessWithoutPassword
	PermAddOfflineDownload
	PermWrite
	PermRename
	PermMove
	PermCopy
	PermRemove
	PermWebdavRead
	PermWebdavManage
	PermFTPAccess
	PermFTPManage
	PermReadArchives
	PermDecompress
	PermPathLimit
)

func HasPermission(perm int32, bit uint) bool {
	return (perm>>bit)&1 == 1
}

func MergeRolePermissions(u *model.User, reqPath string) int32 {
	if u == nil {
		return 0
	}
	var perm int32
	for _, rid := range u.Role {
		role, err := op.GetRole(uint(rid))
		if err != nil {
			continue
		}
		if reqPath == "/" || utils.PathEqual(reqPath, u.BasePath) {
			for _, entry := range role.PermissionScopes {
				perm |= entry.Permission
			}
		} else {
			for _, entry := range role.PermissionScopes {
				if utils.IsSubPath(entry.Path, reqPath) {
					perm |= entry.Permission
				}
			}
		}
	}
	return perm
}

func CanAccessWithRoles(u *model.User, meta *model.Meta, reqPath, password string) bool {
	if !CanReadPathByRole(u, reqPath) {
		return false
	}
	perm := MergeRolePermissions(u, reqPath)
	if meta != nil && !HasPermission(perm, PermSeeHides) && meta.Hide != "" &&
		IsApply(meta.Path, path.Dir(reqPath), meta.HSub) {
		for _, hide := range strings.Split(meta.Hide, "\n") {
			re := regexp2.MustCompile(hide, regexp2.None)
			if isMatch, _ := re.MatchString(path.Base(reqPath)); isMatch {
				return false
			}
		}
	}
	if HasPermission(perm, PermAccessWithoutPassword) {
		return true
	}
	if meta == nil || meta.Password == "" {
		return true
	}
	if !utils.PathEqual(meta.Path, reqPath) && !meta.PSub {
		return true
	}
	return meta.Password == password
}

func CanReadPathByRole(u *model.User, reqPath string) bool {
	if u == nil {
		return false
	}
	if reqPath == "/" || utils.PathEqual(reqPath, u.BasePath) {
		return len(u.Role) > 0
	}
	for _, rid := range u.Role {
		role, err := op.GetRole(uint(rid))
		if err != nil {
			continue
		}
		for _, entry := range role.PermissionScopes {
			if utils.PathEqual(entry.Path, reqPath) || utils.IsSubPath(entry.Path, reqPath) || utils.IsSubPath(reqPath, entry.Path) {
				return true
			}
		}
	}
	return false
}

// HasChildPermission checks whether any child path under reqPath grants the
// specified permission bit.
func HasChildPermission(u *model.User, reqPath string, bit uint) bool {
	if u == nil {
		return false
	}
	for _, rid := range u.Role {
		role, err := op.GetRole(uint(rid))
		if err != nil {
			continue
		}
		for _, entry := range role.PermissionScopes {
			if utils.IsSubPath(reqPath, entry.Path) && HasPermission(entry.Permission, bit) {
				return true
			}
		}
	}
	return false
}

// CheckPathLimitWithRoles checks whether the path is allowed when the user has
// the `PermPathLimit` permission for the target path. When the user does not
// have this permission, the check passes by default.
func CheckPathLimitWithRoles(u *model.User, reqPath string) bool {
	perm := MergeRolePermissions(u, reqPath)
	if HasPermission(perm, PermPathLimit) {
		return CanReadPathByRole(u, reqPath)
	}
	return true
}
