package webdav

import (
	"path"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

// ResolvePath normalizes the provided raw path and resolves it against the user's base path
// before delegating to the user-aware JoinPath permission checks.
func ResolvePath(user *model.User, raw string) (string, error) {
	cleaned := utils.FixAndCleanPath(raw)
	basePath := utils.FixAndCleanPath(user.BasePath)

	if cleaned != "/" && basePath != "/" && !utils.IsSubPath(basePath, cleaned) {
		cleaned = path.Join(basePath, strings.TrimPrefix(cleaned, "/"))
	}

	return user.JoinPath(cleaned)
}
