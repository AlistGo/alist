package lark

import (
	"context"
	"github.com/Xhofe/go-cache"
	log "github.com/sirupsen/logrus"
	"path"
	"strings"
	"time"
)

const objTokenCacheDuration = 5 * time.Minute
const emptyFolderToken = "empty"

var objTokenCache = cache.NewMemCache[string]()
var exOpts = cache.WithEx[string](objTokenCacheDuration)

func (c *Lark) getObjToken(ctx context.Context, folderPath string) (string, bool) {
	if token, ok := objTokenCache.Get(folderPath); ok {
		return token, true
	}

	dir, name := path.Split(folderPath)
	// strip the last slash of dir if it exists
	if len(dir) > 0 && dir[len(dir)-1] == '/' {
		dir = dir[:len(dir)-1]
	}
	if name == "" {
		return c.rootFolderToken, true
	}

	var parentToken string
	var found bool
	parentToken, found = c.getObjToken(ctx, dir)
	if !found {
		return emptyFolderToken, false
	}

	files, err := c.listFiles(ctx, parentToken)
	if err != nil {
		log.WithError(err).Error("failed to list files")
		return emptyFolderToken, false
	}

	for _, file := range files {
		if *file.Name == name || *file.Name == trimLarkDisplayExt(name) {
			objTokenCache.Set(folderPath, *file.Token, exOpts)
			return *file.Token, true
		}
	}

	return emptyFolderToken, false
}

func trimLarkDisplayExt(name string) string {
	for _, suffix := range larkCloudDocSuffixes() {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func isLarkCloudDocName(name string) bool {
	for _, suffix := range larkCloudDocSuffixes() {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func larkCloudDocSuffixes() []string {
	return []string{
		".lark-doc",
		".lark-docx",
		".lark-sheet",
		".lark-bitable",
		".lark-mindnote",
		".lark-slides",
	}
}
