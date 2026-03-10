package strm

import (
	"context"
	"errors"
	stdpath "path"
	"path/filepath"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	log "github.com/sirupsen/logrus"
)

type Strm struct {
	model.Storage
	Addition

	aliases       map[string][]string
	autoFlatten   bool
	singleRootKey string

	mediaExtSet      map[string]struct{}
	downloadExtSet   map[string]struct{}
	normalizedMode   string
	normalizedPrefix string
}

func (d *Strm) Config() driver.Config {
	return config
}

func (d *Strm) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Strm) Init(ctx context.Context) error {
	if strings.TrimSpace(d.Paths) == "" {
		return errors.New("paths is required")
	}
	if d.SaveStrmToLocal && strings.TrimSpace(d.SaveStrmLocalPath) == "" {
		return errors.New("SaveStrmLocalPath is required")
	}

	d.aliases = parseAliases(d.Paths)
	if len(d.aliases) == 0 {
		return errors.New("no valid path mapping found")
	}

	d.autoFlatten = len(d.aliases) == 1
	d.singleRootKey = ""
	if d.autoFlatten {
		for k := range d.aliases {
			d.singleRootKey = k
		}
	}

	d.mediaExtSet = parseExtSet(defaultIfEmpty(d.FilterFileTypes, defaultMediaExt))
	d.downloadExtSet = parseExtSet(defaultIfEmpty(d.DownloadFileTypes, defaultDownloadExt))
	d.normalizedPrefix = normalizePrefix(defaultIfEmpty(d.PathPrefix, "/d"))
	d.normalizedMode = normalizeSaveMode(d.SaveLocalMode)

	if d.Version != 5 {
		d.FilterFileTypes = mergeDefaultExtCSV(d.FilterFileTypes, defaultMediaExt)
		d.DownloadFileTypes = mergeDefaultExtCSV(d.DownloadFileTypes, defaultDownloadExt)
		d.PathPrefix = "/d"
		d.Version = 5
	}
	if d.SaveLocalMode == "" {
		d.SaveLocalMode = SaveLocalInsertMode
	}
	if d.SignExpireHours < 0 {
		d.SignExpireHours = 0
	}
	if d.RotateSignNow {
		d.RotateSignNow = false
		op.MustSaveDriverStorage(d)
		if d.SaveStrmToLocal && strings.TrimSpace(d.SaveStrmLocalPath) != "" {
			go func() {
				log.Infof("strm: start rotating signs for [%s]", d.MountPath)
				d.rotateAllLocal(context.Background())
				log.Infof("strm: finished rotating signs for [%s]", d.MountPath)
			}()
		}
	}
	return nil
}

func (d *Strm) Drop(ctx context.Context) error {
	d.aliases = nil
	d.mediaExtSet = nil
	d.downloadExtSet = nil
	return nil
}

func (Addition) GetRootPath() string {
	return "/"
}

func (d *Strm) Get(ctx context.Context, path string) (model.Obj, error) {
	path = cleanPath(path)
	root, sub := d.splitVirtualPath(path)
	targets, ok := d.aliases[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}

	for _, targetRoot := range targets {
		realPath := stdpath.Join(targetRoot, sub)
		obj, err := fs.Get(ctx, realPath, &fs.GetArgs{NoLog: true})
		if err != nil {
			continue
		}
		if obj.IsDir() {
			return wrapObj(path, obj, 0), nil
		}
		return wrapObj(realPath, obj, obj.GetSize()), nil
	}

	if strings.HasSuffix(strings.ToLower(path), ".strm") {
		return nil, errs.NotSupport
	}
	return nil, errs.ObjectNotFound
}

func (d *Strm) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	virtualDir := cleanPath(dir.GetPath())
	if virtualDir == "/" && !d.autoFlatten {
		objs := d.listVirtualRoots()
		d.syncLocalDir(ctx, virtualDir, objs)
		return objs, nil
	}

	root, sub := d.splitVirtualPath(virtualDir)
	targets, ok := d.aliases[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}

	out := make([]model.Obj, 0)
	for _, targetRoot := range targets {
		realDir := stdpath.Join(targetRoot, sub)
		objs, err := fs.List(ctx, realDir, &fs.ListArgs{NoLog: true, Refresh: args.Refresh})
		if err != nil {
			continue
		}
		out = append(out, d.mapListedObjects(ctx, realDir, objs)...)
	}

	d.syncLocalDir(ctx, virtualDir, out)
	return out, nil
}

func (d *Strm) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.GetID() == "strm" {
		line := d.buildStrmLine(ctx, file.GetPath())
		return &model.Link{MFile: model.NewNopMFile(strings.NewReader(line + "\n"))}, nil
	}
	return d.linkRealFile(ctx, file.GetPath(), args)
}

func (d *Strm) listVirtualRoots() []model.Obj {
	objs := make([]model.Obj, 0, len(d.aliases))
	for k := range d.aliases {
		objs = append(objs, &model.Object{
			Path:     "/" + k,
			Name:     k,
			IsFolder: true,
			Modified: d.Modified,
		})
	}
	return objs
}

func (d *Strm) rotateAllLocal(ctx context.Context) {
	for alias, roots := range d.aliases {
		virtualRoot := "/"
		if !d.autoFlatten {
			virtualRoot = "/" + alias
		}
		for _, realRoot := range roots {
			d.walkAndSync(ctx, virtualRoot, realRoot)
		}
	}
}

func (d *Strm) walkAndSync(ctx context.Context, virtualDir, realDir string) {
	objs, err := fs.List(ctx, realDir, &fs.ListArgs{NoLog: true, Refresh: true})
	if err != nil {
		log.Warnf("strm: rotate list failed %s: %v", realDir, err)
		return
	}
	mapped := d.mapListedObjects(ctx, realDir, objs)
	d.syncLocalDirWithMode(ctx, virtualDir, mapped, SaveLocalUpdateMode)
	for _, obj := range objs {
		if !obj.IsDir() {
			continue
		}
		childVirtual := stdpath.Join(virtualDir, obj.GetName())
		childReal := stdpath.Join(realDir, obj.GetName())
		d.walkAndSync(ctx, childVirtual, childReal)
	}
}

func (d *Strm) mapListedObjects(ctx context.Context, realDir string, listed []model.Obj) []model.Obj {
	ret := make([]model.Obj, 0, len(listed))
	for _, obj := range listed {
		if obj.IsDir() {
			ret = append(ret, &model.Object{
				Name:     obj.GetName(),
				Path:     "",
				IsFolder: true,
				Modified: obj.ModTime(),
			})
			continue
		}

		realPath := stdpath.Join(realDir, obj.GetName())
		ext := fileExt(obj.GetName())

		if _, ok := d.downloadExtSet[ext]; ok {
			ret = append(ret, d.cloneWithPath(obj, realPath, obj.GetName(), "", obj.GetSize()))
			continue
		}
		if _, ok := d.mediaExtSet[ext]; ok {
			strmName := strings.TrimSuffix(obj.GetName(), stdpath.Ext(obj.GetName())) + ".strm"
			size := int64(len(d.buildStrmLine(ctx, realPath)) + 1)
			ret = append(ret, d.cloneWithPath(obj, realPath, strmName, "strm", size))
		}
	}
	return ret
}

func (d *Strm) cloneWithPath(src model.Obj, realPath, name, id string, size int64) model.Obj {
	baseObj := model.Object{
		ID:       id,
		Path:     realPath,
		Name:     name,
		Size:     size,
		Modified: src.ModTime(),
		IsFolder: src.IsDir(),
	}
	thumb, ok := model.GetThumb(src)
	if !ok {
		return &baseObj
	}
	return &model.ObjThumb{Object: baseObj, Thumbnail: model.Thumbnail{Thumbnail: thumb}}
}

func (d *Strm) splitVirtualPath(path string) (string, string) {
	if d.autoFlatten {
		return d.singleRootKey, path
	}
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func cleanPath(path string) string {
	if path == "" {
		return "/"
	}
	return filepath.ToSlash(stdpath.Clean("/" + strings.TrimPrefix(path, "/")))
}

func wrapObj(path string, src model.Obj, size int64) model.Obj {
	return &model.Object{
		Path:     path,
		Name:     src.GetName(),
		Size:     size,
		Modified: src.ModTime(),
		IsFolder: src.IsDir(),
		HashInfo: src.GetHash(),
	}
}

var _ driver.Driver = (*Strm)(nil)
