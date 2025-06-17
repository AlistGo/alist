package utils

import (
	"net/url"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/errs"
)

// FixAndCleanPath
// The upper layer of the root directory is still the root directory.
// So ".." And "." will be cleared
// for example
// 1. ".." or "." => "/"
// 2. "../..." or "./..." => "/..."
// 3. "../.x." or "./.x." => "/.x."
// 4. "x//\\y" = > "/z/x"
func FixAndCleanPath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return stdpath.Clean(path)
}

// PathAddSeparatorSuffix Add path '/' suffix
// for example /root => /root/
func PathAddSeparatorSuffix(path string) string {
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}
	return path
}

// PathEqual judge path is equal
func PathEqual(path1, path2 string) bool {
	return FixAndCleanPath(path1) == FixAndCleanPath(path2)
}

func IsSubPath(path string, subPath string) bool {
	path, subPath = FixAndCleanPath(path), FixAndCleanPath(subPath)
	return path == subPath || strings.HasPrefix(subPath, PathAddSeparatorSuffix(path))
}

func Ext(path string) string {
	ext := stdpath.Ext(path)
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return strings.ToLower(ext)
}

func EncodePath(path string, all ...bool) string {
	seg := strings.Split(path, "/")
	toReplace := []struct {
		Src string
		Dst string
	}{
		{Src: "%", Dst: "%25"},
		{"%", "%25"},
		{"?", "%3F"},
		{"#", "%23"},
	}
	for i := range seg {
		if len(all) > 0 && all[0] {
			seg[i] = url.PathEscape(seg[i])
		} else {
			for j := range toReplace {
				seg[i] = strings.ReplaceAll(seg[i], toReplace[j].Src, toReplace[j].Dst)
			}
		}
	}
	return strings.Join(seg, "/")
}

func JoinBasePath(basePath, reqPath string) (string, error) {
	/** relative path:
	 * 1. ..
	 * 2. ../
	 * 3. /..
	 * 4. /../
	 * 5. /a/b/..
	 */
	if reqPath == ".." ||
		strings.HasSuffix(reqPath, "/..") ||
		strings.HasPrefix(reqPath, "../") ||
		strings.Contains(reqPath, "/../") {
		return "", errs.RelativePath
	}
	return stdpath.Join(FixAndCleanPath(basePath), FixAndCleanPath(reqPath)), nil
}

func GetFullPath(mountPath, path string) string {
	return stdpath.Join(GetActualMountPath(mountPath), path)
}
