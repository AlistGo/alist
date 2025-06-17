package op

import (
	"context"
	stdpath "path"
	"slices"
	"time"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/generic_sync"
	"github.com/alist-org/alist/v3/pkg/singleflight"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// In order to facilitate adding some other things before and after file op

var listCache = cache.NewMemCache(cache.WithShards[[]model.Obj](64))
var listG singleflight.Group[[]model.Obj]

func updateCacheObj(storage driver.Driver, path string, oldObj model.Obj, newObj model.Obj) {
	key := Key(storage, path)
	objs, ok := listCache.Get(key)
	if ok {
		for i, obj := range objs {
			if obj.GetName() == newObj.GetName() {
				objs = slices.Delete(objs, i, i+1)
				break
			}
		}
		for i, obj := range objs {
			if obj.GetName() == oldObj.GetName() {
				objs[i] = newObj
				break
			}
		}
		listCache.Set(key, objs, cache.WithEx[[]model.Obj](time.Minute*time.Duration(storage.GetStorage().CacheExpiration)))
	}
}

func delCacheObj(storage driver.Driver, path string, obj model.Obj) {
	key := Key(storage, path)
	objs, ok := listCache.Get(key)
	if ok {
		for i, oldObj := range objs {
			if oldObj.GetName() == obj.GetName() {
				objs = append(objs[:i], objs[i+1:]...)
				break
			}
		}
		listCache.Set(key, objs, cache.WithEx[[]model.Obj](time.Minute*time.Duration(storage.GetStorage().CacheExpiration)))
	}
}

var addSortDebounceMap generic_sync.MapOf[string, func(func())]

func addCacheObj(storage driver.Driver, path string, newObj model.Obj) {
	key := Key(storage, path)
	objs, ok := listCache.Get(key)
	if ok {
		for i, obj := range objs {
			if obj.GetName() == newObj.GetName() {
				objs[i] = newObj
				return
			}
		}

		// Simple separation of files and folders
		if len(objs) > 0 && objs[len(objs)-1].IsDir() == newObj.IsDir() {
			objs = append(objs, newObj)
		} else {
			objs = append([]model.Obj{newObj}, objs...)
		}

		if storage.Config().LocalSort {
			debounce, _ := addSortDebounceMap.LoadOrStore(key, utils.NewDebounce(time.Minute))
			log.Debug("addCacheObj: wait start sort")
			debounce(func() {
				log.Debug("addCacheObj: start sort")
				model.SortFiles(objs, storage.GetStorage().OrderBy, storage.GetStorage().OrderDirection)
				addSortDebounceMap.Delete(key)
			})
		}

		listCache.Set(key, objs, cache.WithEx[[]model.Obj](time.Minute*time.Duration(storage.GetStorage().CacheExpiration)))
	}
}

func ClearCache(storage driver.Driver, path string) {
	objs, ok := listCache.Get(Key(storage, path))
	if ok {
		for _, obj := range objs {
			if obj.IsDir() {
				ClearCache(storage, stdpath.Join(path, obj.GetName()))
			}
		}
	}
	listCache.Del(Key(storage, path))
}

func Key(storage driver.Driver, path string) string {
	return stdpath.Join(storage.GetStorage().MountPath, utils.FixAndCleanPath(path))
}

// List files in storage, not contains virtual file
func List(ctx context.Context, storage driver.Driver, path string, args model.ListArgs) ([]model.Obj, error) {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return nil, errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	path = utils.FixAndCleanPath(path)
	log.Debugf("op.List %s", path)
	key := Key(storage, path)
	if !args.Refresh {
		if files, ok := listCache.Get(key); ok {
			log.Debugf("use cache when list %s", path)
			return files, nil
		}
	}
	dir, err := GetUnwrap(ctx, storage, path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get dir")
	}
	log.Debugf("list dir: %+v", dir)
	if !dir.IsDir() {
		return nil, errors.WithStack(errs.NotFolder)
	}
	objs, err, _ := listG.Do(key, func() ([]model.Obj, error) {
		files, err := storage.List(ctx, dir, args)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list objs")
		}
		// set path
		for _, f := range files {
			if s, ok := f.(model.SetPath); ok && f.GetPath() == "" && dir.GetPath() != "" {
				s.SetPath(stdpath.Join(dir.GetPath(), f.GetName()))
			}
		}
		// warp obj name
		model.WrapObjsName(files)
		// call hooks
		go func(reqPath string, files []model.Obj) {
			HandleObjsUpdateHook(reqPath, files)
		}(utils.GetFullPath(storage.GetStorage().MountPath, path), files)

		// sort objs
		if storage.Config().LocalSort {
			model.SortFiles(files, storage.GetStorage().OrderBy, storage.GetStorage().OrderDirection)
		}
		model.ExtractFolder(files, storage.GetStorage().ExtractFolder)

		if !storage.Config().NoCache {
			if len(files) > 0 {
				log.Debugf("set cache: %s => %+v", key, files)
				listCache.Set(key, files, cache.WithEx[[]model.Obj](time.Minute*time.Duration(storage.GetStorage().CacheExpiration)))
			} else {
				log.Debugf("del cache: %s", key)
				listCache.Del(key)
			}
		}
		return files, nil
	})
	return objs, err
}

// Get object from list of files
func Get(ctx context.Context, storage driver.Driver, path string) (model.Obj, error) {
	path = utils.FixAndCleanPath(path)
	log.Debugf("op.Get %s", path)

	// get the obj directly without list so that we can reduce the io
	if g, ok := storage.(driver.Getter); ok {
		obj, err := g.Get(ctx, path)
		if err == nil {
			return model.WrapObjName(obj), nil
		}
	}

	// is root folder
	if utils.PathEqual(path, "/") {
		var rootObj model.Obj
		if getRooter, ok := storage.(driver.GetRooter); ok {
			obj, err := getRooter.GetRoot(ctx)
			if err != nil {
				return nil, errors.WithMessage(err, "failed get root obj")
			}
			rootObj = obj
		} else {
			switch r := storage.GetAddition().(type) {
			case driver.IRootId:
				rootObj = &model.Object{
					ID:       r.GetRootId(),
					Name:     RootName,
					Size:     0,
					Modified: storage.GetStorage().Modified,
					IsFolder: true,
				}
			case driver.IRootPath:
				rootObj = &model.Object{
					Path:     r.GetRootPath(),
					Name:     RootName,
					Size:     0,
					Modified: storage.GetStorage().Modified,
					IsFolder: true,
				}
			default:
				return nil, errors.Errorf("please implement IRootPath or IRootId or GetRooter method")
			}
		}
		if rootObj == nil {
			return nil, errors.Errorf("please implement IRootPath or IRootId or GetRooter method")
		}
		return &model.ObjWrapName{
			Name: RootName,
			Obj:  rootObj,
		}, nil
	}

	// not root folder
	dir, name := stdpath.Split(path)
	files, err := List(ctx, storage, dir, model.ListArgs{})
	if err != nil {
		return nil, errors.WithMessage(err, "failed get parent list")
	}
	for _, f := range files {
		if f.GetName() == name {
			return f, nil
		}
	}
	log.Debugf("cant find obj with name: %s", name)
	return nil, errors.WithStack(errs.ObjectNotFound)
}

func GetUnwrap(ctx context.Context, storage driver.Driver, path string) (model.Obj, error) {
	obj, err := Get(ctx, storage, path)
	if err != nil {
		return nil, err
	}
	return model.UnwrapObj(obj), err
}

var linkCache = cache.NewMemCache(cache.WithShards[*model.Link](16))
var linkG singleflight.Group[*model.Link]

// Link get link, if is an url. should have an expiry time
func Link(ctx context.Context, storage driver.Driver, path string, args model.LinkArgs) (*model.Link, model.Obj, error) {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return nil, nil, errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	file, err := GetUnwrap(ctx, storage, path)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "failed to get file")
	}
	if file.IsDir() {
		return nil, nil, errors.WithStack(errs.NotFile)
	}
	key := Key(storage, path)
	if link, ok := linkCache.Get(key); ok {
		return link, file, nil
	}
	fn := func() (*model.Link, error) {
		link, err := storage.Link(ctx, file, args)
		if err != nil {
			return nil, errors.Wrapf(err, "failed get link")
		}
		if link.Expiration != nil {
			if link.IPCacheKey {
				key = key + ":" + args.IP
			}
			linkCache.Set(key, link, cache.WithEx[*model.Link](*link.Expiration))
		}
		return link, nil
	}

	if storage.Config().OnlyLocal {
		link, err := fn()
		return link, file, err
	}

	link, err, _ := linkG.Do(key, fn)
	return link, file, err
}

// Other api
func Other(ctx context.Context, storage driver.Driver, args model.FsOtherArgs) (interface{}, error) {
	obj, err := GetUnwrap(ctx, storage, args.Path)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to get obj")
	}
	if o, ok := storage.(driver.Other); ok {
		return o.Other(ctx, model.OtherArgs{
			Obj:    obj,
			Method: args.Method,
			Data:   args.Data,
		})
	} else {
		return nil, errs.NotImplement
	}
}

var mkdirG singleflight.Group[interface{}]

func MakeDir(ctx context.Context, storage driver.Driver, path string, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	path = utils.FixAndCleanPath(path)
	key := Key(storage, path)
	_, err, _ := mkdirG.Do(key, func() (interface{}, error) {
		// check if dir exists
		f, err := GetUnwrap(ctx, storage, path)
		if err != nil {
			if errs.IsObjectNotFound(err) {
				parentPath, dirName := stdpath.Split(path)
				err = MakeDir(ctx, storage, parentPath)
				if err != nil {
					return nil, errors.WithMessagef(err, "failed to make parent dir [%s]", parentPath)
				}
				parentDir, err := GetUnwrap(ctx, storage, parentPath)
				// this should not happen
				if err != nil {
					return nil, errors.WithMessagef(err, "failed to get parent dir [%s]", parentPath)
				}

				switch s := storage.(type) {
				case driver.MkdirResult:
					var newObj model.Obj
					newObj, err = s.MakeDir(ctx, parentDir, dirName)
					if err == nil {
						if newObj != nil {
							addCacheObj(storage, parentPath, model.WrapObjName(newObj))
						} else if !utils.IsBool(lazyCache...) {
							ClearCache(storage, parentPath)
						}
					}
				case driver.Mkdir:
					err = s.MakeDir(ctx, parentDir, dirName)
					if err == nil && !utils.IsBool(lazyCache...) {
						ClearCache(storage, parentPath)
					}
				default:
					return nil, errs.NotImplement
				}
				return nil, errors.WithStack(err)
			}
			return nil, errors.WithMessage(err, "failed to check if dir exists")
		}
		// dir exists
		if f.IsDir() {
			return nil, nil
		}
		// dir to make is a file
		return nil, errors.New("file exists")
	})
	return err
}

func Move(ctx context.Context, storage driver.Driver, srcPath, dstDirPath string, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	srcPath = utils.FixAndCleanPath(srcPath)
	dstDirPath = utils.FixAndCleanPath(dstDirPath)
	srcRawObj, err := Get(ctx, storage, srcPath)
	if err != nil {
		return errors.WithMessage(err, "failed to get src object")
	}
	srcObj := model.UnwrapObj(srcRawObj)
	dstDir, err := GetUnwrap(ctx, storage, dstDirPath)
	if err != nil {
		return errors.WithMessage(err, "failed to get dst dir")
	}
	srcDirPath := stdpath.Dir(srcPath)

	switch s := storage.(type) {
	case driver.MoveResult:
		var newObj model.Obj
		newObj, err = s.Move(ctx, srcObj, dstDir)
		if err == nil {
			delCacheObj(storage, srcDirPath, srcRawObj)
			if newObj != nil {
				addCacheObj(storage, dstDirPath, model.WrapObjName(newObj))
			} else if !utils.IsBool(lazyCache...) {
				ClearCache(storage, dstDirPath)
			}
		}
	case driver.Move:
		err = s.Move(ctx, srcObj, dstDir)
		if err == nil {
			delCacheObj(storage, srcDirPath, srcRawObj)
			if !utils.IsBool(lazyCache...) {
				ClearCache(storage, dstDirPath)
			}
		}
	default:
		return errs.NotImplement
	}
	return errors.WithStack(err)
}

func Rename(ctx context.Context, storage driver.Driver, srcPath, dstName string, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	srcPath = utils.FixAndCleanPath(srcPath)
	srcRawObj, err := Get(ctx, storage, srcPath)
	if err != nil {
		return errors.WithMessage(err, "failed to get src object")
	}
	srcObj := model.UnwrapObj(srcRawObj)
	srcDirPath := stdpath.Dir(srcPath)

	switch s := storage.(type) {
	case driver.RenameResult:
		var newObj model.Obj
		newObj, err = s.Rename(ctx, srcObj, dstName)
		if err == nil {
			if newObj != nil {
				updateCacheObj(storage, srcDirPath, srcRawObj, model.WrapObjName(newObj))
			} else if !utils.IsBool(lazyCache...) {
				ClearCache(storage, srcDirPath)
			}
		}
	case driver.Rename:
		err = s.Rename(ctx, srcObj, dstName)
		if err == nil && !utils.IsBool(lazyCache...) {
			ClearCache(storage, srcDirPath)
		}
	default:
		return errs.NotImplement
	}
	return errors.WithStack(err)
}

// Copy Just copy file[s] in a storage
func Copy(ctx context.Context, storage driver.Driver, srcPath, dstDirPath string, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	srcPath = utils.FixAndCleanPath(srcPath)
	dstDirPath = utils.FixAndCleanPath(dstDirPath)
	srcObj, err := GetUnwrap(ctx, storage, srcPath)
	if err != nil {
		return errors.WithMessage(err, "failed to get src object")
	}
	dstDir, err := GetUnwrap(ctx, storage, dstDirPath)
	if err != nil {
		return errors.WithMessage(err, "failed to get dst dir")
	}

	switch s := storage.(type) {
	case driver.CopyResult:
		var newObj model.Obj
		newObj, err = s.Copy(ctx, srcObj, dstDir)
		if err == nil {
			if newObj != nil {
				addCacheObj(storage, dstDirPath, model.WrapObjName(newObj))
			} else if !utils.IsBool(lazyCache...) {
				ClearCache(storage, dstDirPath)
			}
		}
	case driver.Copy:
		err = s.Copy(ctx, srcObj, dstDir)
		if err == nil && !utils.IsBool(lazyCache...) {
			ClearCache(storage, dstDirPath)
		}
	default:
		return errs.NotImplement
	}
	return errors.WithStack(err)
}

func Remove(ctx context.Context, storage driver.Driver, path string) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	if utils.PathEqual(path, "/") {
		return errors.New("delete root folder is not allowed, please goto the manage page to delete the storage instead")
	}
	path = utils.FixAndCleanPath(path)
	rawObj, err := Get(ctx, storage, path)
	if err != nil {
		// if object not found, it's ok
		if errs.IsObjectNotFound(err) {
			log.Debugf("%s have been removed", path)
			return nil
		}
		return errors.WithMessage(err, "failed to get object")
	}
	dirPath := stdpath.Dir(path)

	switch s := storage.(type) {
	case driver.Remove:
		err = s.Remove(ctx, model.UnwrapObj(rawObj))
		if err == nil {
			delCacheObj(storage, dirPath, rawObj)
			// clear folder cache recursively
			if rawObj.IsDir() {
				ClearCache(storage, path)
			}
		}
	default:
		return errs.NotImplement
	}
	return errors.WithStack(err)
}

func Put(ctx context.Context, storage driver.Driver, dstDirPath string, file model.FileStreamer, up driver.UpdateProgress, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Errorf("failed to close file streamer, %v", err)
		}
	}()
	// UrlTree PUT
	if storage.GetStorage().Driver == "UrlTree" {
		var link string
		dstDirPath, link = urlTreeSplitLineFormPath(stdpath.Join(dstDirPath, file.GetName()))
		file = &stream.FileStream{Obj: &model.Object{Name: link}}
	}
	// if file exist and size = 0, delete it
	dstDirPath = utils.FixAndCleanPath(dstDirPath)
	dstPath := stdpath.Join(dstDirPath, file.GetName())
	tempName := file.GetName() + ".alist_to_delete"
	tempPath := stdpath.Join(dstDirPath, tempName)
	fi, err := GetUnwrap(ctx, storage, dstPath)
	if err == nil {
		if fi.GetSize() == 0 {
			err = Remove(ctx, storage, dstPath)
			if err != nil {
				return errors.WithMessagef(err, "while uploading, failed remove existing file which size = 0")
			}
		} else if storage.Config().NoOverwriteUpload {
			// try to rename old obj
			err = Rename(ctx, storage, dstPath, tempName)
			if err != nil {
				return err
			}
		} else {
			file.SetExist(fi)
		}
	}
	err = MakeDir(ctx, storage, dstDirPath)
	if err != nil {
		return errors.WithMessagef(err, "failed to make dir [%s]", dstDirPath)
	}
	parentDir, err := GetUnwrap(ctx, storage, dstDirPath)
	// this should not happen
	if err != nil {
		return errors.WithMessagef(err, "failed to get dir [%s]", dstDirPath)
	}
	// if up is nil, set a default to prevent panic
	if up == nil {
		up = func(p float64) {}
	}

	switch s := storage.(type) {
	case driver.PutResult:
		var newObj model.Obj
		newObj, err = s.Put(ctx, parentDir, file, up)
		if err == nil {
			if newObj != nil {
				addCacheObj(storage, dstDirPath, model.WrapObjName(newObj))
			} else if !utils.IsBool(lazyCache...) {
				ClearCache(storage, dstDirPath)
			}
		}
	case driver.Put:
		err = s.Put(ctx, parentDir, file, up)
		if err == nil && !utils.IsBool(lazyCache...) {
			ClearCache(storage, dstDirPath)
		}
	default:
		return errs.NotImplement
	}
	log.Debugf("put file [%s] done", file.GetName())
	if storage.Config().NoOverwriteUpload && fi != nil && fi.GetSize() > 0 {
		if err != nil {
			// upload failed, recover old obj
			err := Rename(ctx, storage, tempPath, file.GetName())
			if err != nil {
				log.Errorf("failed recover old obj: %+v", err)
			}
		} else {
			// upload success, remove old obj
			err := Remove(ctx, storage, tempPath)
			if err != nil {
				return err
			} else {
				key := Key(storage, stdpath.Join(dstDirPath, file.GetName()))
				linkCache.Del(key)
			}
		}
	}
	return errors.WithStack(err)
}

func PutURL(ctx context.Context, storage driver.Driver, dstDirPath, dstName, url string, lazyCache ...bool) error {
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	dstDirPath = utils.FixAndCleanPath(dstDirPath)
	_, err := GetUnwrap(ctx, storage, stdpath.Join(dstDirPath, dstName))
	if err == nil {
		return errors.New("obj already exists")
	}
	err = MakeDir(ctx, storage, dstDirPath)
	if err != nil {
		return errors.WithMessagef(err, "failed to put url")
	}
	dstDir, err := GetUnwrap(ctx, storage, dstDirPath)
	if err != nil {
		return errors.WithMessagef(err, "failed to put url")
	}
	switch s := storage.(type) {
	case driver.PutURLResult:
		var newObj model.Obj
		newObj, err = s.PutURL(ctx, dstDir, dstName, url)
		if err == nil {
			if newObj != nil {
				addCacheObj(storage, dstDirPath, model.WrapObjName(newObj))
			} else if !utils.IsBool(lazyCache...) {
				ClearCache(storage, dstDirPath)
			}
		}
	case driver.PutURL:
		err = s.PutURL(ctx, dstDir, dstName, url)
		if err == nil && !utils.IsBool(lazyCache...) {
			ClearCache(storage, dstDirPath)
		}
	default:
		return errs.NotImplement
	}
	log.Debugf("put url [%s](%s) done", dstName, url)
	return errors.WithStack(err)
}
