//go:build !windows

package local

import (
	"io/fs"
	"strings"
)

func isHidden(f fs.FileInfo) bool {
	return strings.HasPrefix(f.Name(), ".")
}
