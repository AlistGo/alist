package alias

import (
	"context"
	"errors"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type Alias struct {
	model.Storage
	Addition
	pathMap     map[string][]string
	autoFlatten bool
	oneKey      string
}

func (d *Alias) Config() driver.Config {
	return config
}

func (d *Alias) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Alias) Init(ctx context.Context) error {
	if d.Paths == "" {
		return errors.New("paths is required")
	}
	d.pathMap = make(map[string][]string)
	for _, path := range strings.Split(d.Paths, "\n") {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		k, v := getPair(path)
		d.pathMap[k] = append(d.pathMap[k], v)
	}
	if len(d.pathMap) == 1 {
		for k := range d.pathMap {
			d.oneKey = k
		}
		d.autoFlatten = true
	} else {
		d.oneKey = ""
		d.autoFlatten = false
	}
	return nil
}

func (d *Alias) Drop(ctx context.Context) error {
	d.pathMap = nil
	return nil
}

func (d *Alias) Get(ctx context.Context, path string) (model.Obj, error) {
	if utils.PathEqual(path, "/") {
		return &model.Object{
			Name:     "Root",
			IsFolder: true,
			Path:     "/",
		}, nil
	}
	root, sub := d.getRootAndPath(path)
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	for _, dst := range dsts {
		obj, err := d.get(ctx, path, dst, sub)
		if err == nil {
			return obj, nil
		}
	}
	return nil, errs.ObjectNotFound
}

func (d *Alias) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	path := dir.GetPath()
	if utils.PathEqual(path, "/") && !d.autoFlatten {
		return d.listRoot(), nil
	}
	root, sub := d.getRootAndPath(path)
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	var objs []model.Obj
	fsArgs := &fs.ListArgs{NoLog: true, Refresh: args.Refresh}
	for _, dst := range dsts {
		tmp, err := d.list(ctx, dst, sub, fsArgs)
		if err == nil {
			objs = append(objs, tmp...)
		}
	}
	return objs, nil
}

func (d *Alias) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	root, sub := d.getRootAndPath(file.GetPath())
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	for _, dst := range dsts {
		link, err := d.link(ctx, dst, sub, args)
		if err == nil {
			if !args.Redirect && len(link.URL) > 0 {
				// 正常情况下 多并发 仅支持返回URL的驱动
				// alias套娃alias 可以让crypt、mega等驱动(不返回URL的) 支持并发
				if d.DownloadConcurrency > 0 {
					link.Concurrency = d.DownloadConcurrency
				}
				if d.DownloadPartSize > 0 {
					link.PartSize = d.DownloadPartSize * utils.KB
				}
			}
			return link, nil
		}
	}
	return nil, errs.ObjectNotFound
}

func (d *Alias) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	reqPath, err := d.getReqPath(ctx, parentDir, true)
	if err == nil {
		return fs.MakeDir(ctx, stdpath.Join(*reqPath, dirName))
	}
	if errs.IsNotImplement(err) {
		return errors.New("same-name dirs cannot make sub-dir")
	}
	return err
}

func (d *Alias) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	srcPath, err := d.getReqPath(ctx, srcObj, false)
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot be moved")
	}
	if err != nil {
		return err
	}
	dstPath, err := d.getReqPath(ctx, dstDir, true)
	if errs.IsNotImplement(err) {
		return errors.New("same-name dirs cannot be moved to")
	}
	if err != nil {
		return err
	}
	return fs.Move(ctx, *srcPath, *dstPath)
}

func (d *Alias) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	reqPath, err := d.getReqPath(ctx, srcObj, false)
	if err == nil {
		return fs.Rename(ctx, *reqPath, newName)
	}
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot be Rename")
	}
	return err
}

func (d *Alias) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	srcPath, err := d.getReqPath(ctx, srcObj, false)
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot be copied")
	}
	if err != nil {
		return err
	}
	dstPath, err := d.getReqPath(ctx, dstDir, true)
	if errs.IsNotImplement(err) {
		return errors.New("same-name dirs cannot be copied to")
	}
	if err != nil {
		return err
	}
	_, err = fs.Copy(ctx, *srcPath, *dstPath)
	return err
}

func (d *Alias) Remove(ctx context.Context, obj model.Obj) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	reqPath, err := d.getReqPath(ctx, obj, false)
	if err == nil {
		return fs.Remove(ctx, *reqPath)
	}
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot be Delete")
	}
	return err
}

func (d *Alias) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	reqPath, err := d.getReqPath(ctx, dstDir, true)
	if err == nil {
		return fs.PutDirectly(ctx, *reqPath, s)
	}
	if errs.IsNotImplement(err) {
		return errors.New("same-name dirs cannot be Put")
	}
	return err
}

func (d *Alias) PutURL(ctx context.Context, dstDir model.Obj, name, url string) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	reqPath, err := d.getReqPath(ctx, dstDir, true)
	if err == nil {
		return fs.PutURL(ctx, *reqPath, name, url)
	}
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot offline download")
	}
	return err
}

func (d *Alias) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	root, sub := d.getRootAndPath(obj.GetPath())
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	for _, dst := range dsts {
		meta, err := d.getArchiveMeta(ctx, dst, sub, args)
		if err == nil {
			return meta, nil
		}
	}
	return nil, errs.NotImplement
}

func (d *Alias) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	root, sub := d.getRootAndPath(obj.GetPath())
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	for _, dst := range dsts {
		l, err := d.listArchive(ctx, dst, sub, args)
		if err == nil {
			return l, nil
		}
	}
	return nil, errs.NotImplement
}

func (d *Alias) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// alias的两个驱动，一个支持驱动提取，一个不支持，如何兼容？
	// 如果访问的是不支持驱动提取的驱动内的压缩文件，GetArchiveMeta就会返回errs.NotImplement，提取URL前缀就会是/ae，Extract就不会被调用
	// 如果访问的是支持驱动提取的驱动内的压缩文件，GetArchiveMeta就会返回有效值，提取URL前缀就会是/ad，Extract就会被调用
	root, sub := d.getRootAndPath(obj.GetPath())
	dsts, ok := d.pathMap[root]
	if !ok {
		return nil, errs.ObjectNotFound
	}
	for _, dst := range dsts {
		link, err := d.extract(ctx, dst, sub, args)
		if err == nil {
			if !args.Redirect && len(link.URL) > 0 {
				if d.DownloadConcurrency > 0 {
					link.Concurrency = d.DownloadConcurrency
				}
				if d.DownloadPartSize > 0 {
					link.PartSize = d.DownloadPartSize * utils.KB
				}
			}
			return link, nil
		}
	}
	return nil, errs.NotImplement
}

func (d *Alias) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	srcPath, err := d.getReqPath(ctx, srcObj, false)
	if errs.IsNotImplement(err) {
		return errors.New("same-name files cannot be decompressed")
	}
	if err != nil {
		return err
	}
	dstPath, err := d.getReqPath(ctx, dstDir, true)
	if errs.IsNotImplement(err) {
		return errors.New("same-name dirs cannot be decompressed to")
	}
	if err != nil {
		return err
	}
	_, err = fs.ArchiveDecompress(ctx, *srcPath, *dstPath, args)
	return err
}

var _ driver.Driver = (*Alias)(nil)
