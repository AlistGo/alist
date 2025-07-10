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
	perm := u.Permission
	for _, rid := range u.Role {
		role, err := op.GetRole(uint(rid))
		if err != nil {
			continue
		}
		if role.CheckPathLimit() {
			matched := false
			for _, p := range role.BasePaths {
				if utils.IsSubPath(p, reqPath) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		perm |= role.Permission
	}
	return perm
}

func CanAccessWithRoles(u *model.User, meta *model.Meta, reqPath, password string) bool {
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
