package strm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	pkgerr "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	defaultMediaExt    = "mp4,mkv,flv,avi,wmv,ts,rmvb,webm,mp3,flac,aac,wav,ogg,m4a,wma,alac"
	defaultDownloadExt = "ass,srt,vtt,sub,strm"
)

func parseAliases(raw string) map[string][]string {
	aliases := map[string][]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, target := parseAliasLine(line)
		aliases[name] = append(aliases[name], cleanPath(target))
	}
	return aliases
}

func parseAliasLine(line string) (string, string) {
	if strings.Contains(line, ":") {
		parts := strings.SplitN(line, ":", 2)
		if !strings.Contains(parts[0], "/") {
			return parts[0], parts[1]
		}
	}
	return stdpath.Base(line), line
}

func parseExtSet(csv string) map[string]struct{} {
	ret := map[string]struct{}{}
	for _, part := range strings.Split(csv, ",") {
		ext := normalizeExt(part)
		if ext != "" {
			ret[ext] = struct{}{}
		}
	}
	return ret
}

func mergeDefaultExtCSV(csv, defaults string) string {
	base := parseExtSet(csv)
	for ext := range parseExtSet(defaults) {
		base[ext] = struct{}{}
	}
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	ext = strings.TrimPrefix(ext, ".")
	return ext
}

func fileExt(name string) string {
	return normalizeExt(stdpath.Ext(name))
}

func defaultIfEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func normalizeSaveMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "sync":
		return SaveLocalSyncMode
	case "update":
		return SaveLocalUpdateMode
	case "insert", "missing":
		return SaveLocalInsertMode
	default:
		return SaveLocalInsertMode
	}
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/d"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return prefix
}

func (d *Strm) buildStrmLine(ctx context.Context, realPath string) string {
	pathPart := realPath
	if d.EncodePath {
		pathPart = utils.EncodePath(pathPart, true)
	}
	if d.WithSign {
		sep := "?"
		if strings.Contains(pathPart, "?") {
			sep = "&"
		}
		pathPart += sep + "sign=" + d.generateSign(realPath)
	}
	joined := stdpath.Join(d.normalizedPrefix, pathPart)
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	if d.WithoutUrl {
		return joined
	}
	baseURL := strings.TrimSpace(d.SiteUrl)
	if baseURL == "" {
		if c, ok := ctx.(*gin.Context); ok {
			baseURL = common.GetApiUrl(c.Request)
		} else {
			baseURL = common.GetApiUrl(nil)
		}
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	return baseURL + joined
}

func (d *Strm) linkRealFile(ctx context.Context, realPath string, args model.LinkArgs) (*model.Link, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(realPath)
	if err != nil {
		return nil, err
	}
	if !args.Redirect {
		link, _, linkErr := op.Link(ctx, storage, actualPath, args)
		return link, linkErr
	}
	obj, err := fs.Get(ctx, realPath, &fs.GetArgs{NoLog: true})
	if err != nil {
		return nil, err
	}
	if common.ShouldProxy(storage, obj.GetName()) {
		api := common.GetApiUrl(args.HttpReq)
		if api == "" {
			api = strings.TrimSuffix(strings.TrimSpace(d.SiteUrl), "/")
		}
		if api == "" {
			api = common.GetApiUrl(nil)
		}
		return &model.Link{URL: fmt.Sprintf("%s/p%s?sign=%s", api, utils.EncodePath(realPath, true), d.generateSign(realPath))}, nil
	}
	link, _, linkErr := op.Link(ctx, storage, actualPath, args)
	return link, linkErr
}

func (d *Strm) syncLocalDir(ctx context.Context, virtualDir string, objs []model.Obj) {
	d.syncLocalDirWithMode(ctx, virtualDir, objs, d.normalizedMode)
}

func (d *Strm) syncLocalDirWithMode(ctx context.Context, virtualDir string, objs []model.Obj, mode string) {
	if !d.SaveStrmToLocal || strings.TrimSpace(d.SaveStrmLocalPath) == "" {
		return
	}
	baseDir := filepath.Clean(d.SaveStrmLocalPath)
	localDir := baseDir
	if virtualDir != "/" {
		localDir = filepath.Join(baseDir, filepath.FromSlash(strings.TrimPrefix(virtualDir, "/")))
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		log.Warnf("strm: mkdir failed %s: %v", localDir, err)
		return
	}

	expected := map[string]bool{}
	for _, obj := range objs {
		name := obj.GetName()
		expected[name] = obj.IsDir()
		localPath := filepath.Join(localDir, name)
		if obj.IsDir() {
			_ = os.MkdirAll(localPath, 0o755)
			continue
		}
		payload, err := d.localPayload(ctx, obj)
		if err != nil {
			log.Warnf("strm: build local payload failed %s: %v", localPath, err)
			continue
		}
		if err = d.writeLocal(localPath, payload, mode); err != nil {
			log.Warnf("strm: write local failed %s: %v", localPath, err)
		}
	}

	if mode == SaveLocalSyncMode {
		d.syncDeleteExtras(localDir, expected)
	}
}

func (d *Strm) localPayload(ctx context.Context, obj model.Obj) ([]byte, error) {
	if obj.GetID() == "strm" {
		return []byte(d.buildStrmLine(ctx, obj.GetPath()) + "\n"), nil
	}
	link, err := d.linkRealFile(ctx, obj.GetPath(), model.LinkArgs{Redirect: true})
	if err != nil {
		return nil, err
	}
	return readLinkBytes(ctx, link)
}

func readLinkBytes(ctx context.Context, link *model.Link) ([]byte, error) {
	if link.MFile != nil {
		defer link.MFile.Close()
		return io.ReadAll(link.MFile)
	}
	if link.RangeReadCloser != nil {
		rc, err := link.RangeReadCloser.RangeRead(ctx, http_range.Range{Length: -1})
		if err == nil && rc != nil {
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	if link.URL == "" {
		return nil, fmt.Errorf("empty link")
	}
	url := link.URL
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		api := common.GetApiUrl(nil)
		if api == "" {
			return nil, fmt.Errorf("relative url without site url: %s", url)
		}
		url = strings.TrimSuffix(api, "/") + url
	}
	res, err := base.RestyClient.R().SetContext(ctx).SetDoNotParseResponse(true).Get(url)
	if err != nil {
		return nil, err
	}
	defer res.RawBody().Close()
	if res.StatusCode() >= http.StatusBadRequest {
		return nil, fmt.Errorf("read url failed: status=%d", res.StatusCode())
	}
	return io.ReadAll(res.RawBody())
}

func (d *Strm) writeLocal(path string, payload []byte, mode string) error {
	if mode == SaveLocalInsertMode && utils.Exists(path) {
		return nil
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		if mode != SaveLocalSyncMode {
			return nil
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	if mode != SaveLocalInsertMode {
		if old, err := os.ReadFile(path); err == nil {
			if bytes.Equal(old, payload) {
				return nil
			}
		}
	}
	f, err := utils.CreateNestedFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(payload)
	return err
}

func (d *Strm) syncDeleteExtras(localDir string, expected map[string]bool) {
	entries, err := os.ReadDir(localDir)
	if err != nil {
		if pkgerr.Cause(err) != os.ErrNotExist {
			log.Warnf("strm: read local dir failed %s: %v", localDir, err)
		}
		return
	}
	for _, e := range entries {
		expectDir, ok := expected[e.Name()]
		full := filepath.Join(localDir, e.Name())
		if !ok || expectDir != e.IsDir() {
			_ = os.RemoveAll(full)
		}
	}
}

func (d *Strm) generateSign(path string) string {
	if d.SignExpireHours > 0 {
		return sign.WithDuration(path, time.Duration(d.SignExpireHours)*time.Hour)
	}
	return sign.Sign(path)
}
