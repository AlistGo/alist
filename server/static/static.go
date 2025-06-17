package static

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/public"
	"github.com/gin-gonic/gin"
)

var static fs.FS

func initStatic() {
	if conf.Conf.DistDir == "" {
		dist, err := fs.Sub(public.Public, "dist")
		if err != nil {
			utils.Log.Fatalf("failed to read dist dir")
		}
		static = dist
		return
	}
	static = os.DirFS(conf.Conf.DistDir)
}

func initIndex() {
	indexFile, err := static.Open("index.html")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			utils.Log.Fatalf("index.html not exist, you may forget to put dist of frontend to public/dist")
		}
		utils.Log.Fatalf("failed to read index.html: %v", err)
	}
	defer func() {
		_ = indexFile.Close()
	}()
	index, err := io.ReadAll(indexFile)
	if err != nil {
		utils.Log.Fatalf("failed to read dist/index.html")
	}
	conf.RawIndexHtml = string(index)
	siteConfig := getSiteConfig()
	replaceMap := map[string]string{
		"cdn: undefined":       fmt.Sprintf("cdn: '%s'", siteConfig.Cdn),
		"base_path: undefined": fmt.Sprintf("base_path: '%s'", siteConfig.BasePath),
	}
	for k, v := range replaceMap {
		conf.RawIndexHtml = strings.Replace(conf.RawIndexHtml, k, v, 1)
	}
	UpdateIndex()
}

func UpdateIndex() {
	favicon := setting.GetStr(conf.Favicon)
	title := setting.GetStr(conf.SiteTitle)
	customizeHead := setting.GetStr(conf.CustomizeHead)
	customizeBody := setting.GetStr(conf.CustomizeBody)
	mainColor := setting.GetStr(conf.MainColor)
	conf.ManageHtml = conf.RawIndexHtml
	replaceMap1 := map[string]string{
		"https://jsd.nn.ci/gh/alist-org/logo@main/logo.svg": favicon,
		"Loading...":            title,
		"main_color: undefined": fmt.Sprintf("main_color: '%s'", mainColor),
	}
	for k, v := range replaceMap1 {
		conf.ManageHtml = strings.Replace(conf.ManageHtml, k, v, 1)
	}
	conf.IndexHtml = conf.ManageHtml
	replaceMap2 := map[string]string{
		"<!-- customize head -->": customizeHead,
		"<!-- customize body -->": customizeBody,
	}
	for k, v := range replaceMap2 {
		conf.IndexHtml = strings.Replace(conf.IndexHtml, k, v, 1)
	}
}

func Static(r *gin.RouterGroup, noRoute func(handlers ...gin.HandlerFunc)) {
	initStatic()
	initIndex()
	folders := []string{"assets", "images", "streamer", "static"}
	r.Use(func(c *gin.Context) {
		for i := range folders {
			if strings.HasPrefix(c.Request.RequestURI, fmt.Sprintf("/%s/", folders[i])) {
				c.Header("Cache-Control", "public, max-age=15552000")
			}
		}
	})
	for i, folder := range folders {
		sub, err := fs.Sub(static, folder)
		if err != nil {
			utils.Log.Fatalf("can't find folder: %s", folder)
		}
		r.StaticFS(fmt.Sprintf("/%s/", folders[i]), http.FS(sub))
	}

	noRoute(func(c *gin.Context) {
		if c.Request.Method != "GET" && c.Request.Method != "POST" {
			c.Status(405)
			return
		}
		c.Header("Content-Type", "text/html")
		c.Status(200)
		if strings.HasPrefix(c.Request.URL.Path, "/@manage") {
			_, _ = c.Writer.WriteString(conf.ManageHtml)
		} else {
			_, _ = c.Writer.WriteString(conf.IndexHtml)
		}
		c.Writer.Flush()
		c.Writer.WriteHeaderNow()
	})
}
