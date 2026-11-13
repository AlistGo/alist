//go:build windows

package local

import (
	"io/fs"
	"strings"
	"syscall"
)

// isHidden also checks the hidden attribute, because on Windows that is what
// hides entries such as $RECYCLE.BIN and System Volume Information.
func isHidden(f fs.FileInfo) bool {
	if strings.HasPrefix(f.Name(), ".") {
		return true
	}
	if attrs, ok := f.Sys().(*syscall.Win32FileAttributeData); ok {
		return attrs.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
	}
	return false
}
