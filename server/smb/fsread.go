package smb

import (
	"context"
	"hash/fnv"
	"os"

	"github.com/KirCute/go-smb2-alist/vfs"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/pkg/errors"
)

type readingFile struct {
	path    string
	obj     model.Obj
	s       stream.SStreamReadAtSeeker
	dirRead bool
}

func newRead(path string, obj model.Obj) *readingFile {
	return &readingFile{path: utils.FixAndCleanPath(path), obj: obj, s: nil}
}

func (f *readingFile) initDownload(ctx context.Context) error {
	if f.s != nil {
		return nil
	}
	if f.obj.IsDir() {
		return errs.NotFile
	}
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(f.path)
	if err != nil {
		return err
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return err
		}
	}
	ctx = context.WithValue(ctx, "meta", meta)
	if !common.CanAccess(user, meta, reqPath, "") {
		return errs.PermissionDenied
	}

	link, obj, err := fs.Link(ctx, reqPath, model.LinkArgs{})
	if err != nil {
		return err
	}
	f.obj = obj
	fileStream := stream.FileStream{
		Obj: obj,
		Ctx: ctx,
	}
	ss, err := stream.NewSeekableStream(fileStream, link)
	if err != nil {
		return err
	}
	reader, err := stream.NewReadAtSeeker(ss, 0)
	if err != nil {
		_ = ss.Close()
		return err
	}
	f.s = reader
	return nil
}

func (f *readingFile) Close() error {
	if f.s == nil {
		return nil
	}
	return f.s.Close()
}

func FsGet(ctx context.Context, path string) (model.Obj, error) {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return nil, err
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return nil, err
		}
	}
	ctx = context.WithValue(ctx, "meta", meta)
	if !common.CanAccess(user, meta, reqPath, "") {
		return nil, errs.PermissionDenied
	}
	return fs.Get(ctx, reqPath, &fs.GetArgs{})
}

func List(ctx context.Context, path string) ([]vfs.DirInfo, error) {
	user := ctx.Value("user").(*model.User)
	reqPath, err := user.JoinPath(path)
	if err != nil {
		return nil, err
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return nil, err
		}
	}
	ctx = context.WithValue(ctx, "meta", meta)
	if !common.CanAccess(user, meta, reqPath, "") {
		return nil, errs.PermissionDenied
	}
	self, err := fs.Get(ctx, reqPath, &fs.GetArgs{})
	if err != nil {
		return nil, err
	}
	attr, err := MakeObjAttribute(self)
	if err != nil {
		return nil, err
	}
	objs, err := fs.List(ctx, reqPath, &fs.ListArgs{})
	if err != nil {
		return nil, err
	}
	ret := make([]vfs.DirInfo, 0, len(objs)+2)
	ret = append(ret, vfs.DirInfo{Name: ".", Attributes: *attr})
	if utils.FixAndCleanPath(user.BasePath) != reqPath {
		ret = append(ret, vfs.DirInfo{Name: "..", Attributes: *attr})
	}
	for _, obj := range objs {
		a, e := MakeObjAttribute(obj)
		if e != nil {
			continue
		}
		ret = append(ret, vfs.DirInfo{Name: obj.GetName(), Attributes: *a})
	}
	return ret, nil
}

func MakeFileAttribute(file *os.File) (*vfs.Attributes, error) {
	a := &vfs.Attributes{}
	a.SetInodeNumber(uint64(file.Fd())) // 用fd没什么依据，纯随便给了个不会冲突的整数
	stat, err := file.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a.SetSizeBytes(uint64(stat.Size()))
	a.SetUnixMode(uint32(stat.Mode()))
	a.SetPermissions(vfs.NewPermissionsFromMode(uint32(stat.Mode().Perm())))
	a.SetLastDataModificationTime(stat.ModTime())
	if stat.IsDir() {
		a.SetFileType(vfs.FileTypeDirectory)
	} else {
		a.SetFileType(vfs.FileTypeRegularFile)
	}
	return a, nil
}

func MakeObjAttribute(obj model.Obj) (*vfs.Attributes, error) {
	a := &vfs.Attributes{}
	h := fnv.New64()
	_, err := h.Write([]byte(obj.GetPath()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	a.SetInodeNumber(h.Sum64())
	a.SetSizeBytes(uint64(obj.GetSize()))
	a.SetLastDataModificationTime(obj.ModTime())
	if obj.IsDir() {
		a.SetFileType(vfs.FileTypeDirectory)
		a.SetUnixMode(0755)
		a.SetPermissions(vfs.NewPermissionsFromMode(0755))
	} else {
		a.SetFileType(vfs.FileTypeRegularFile)
		a.SetUnixMode(0644)
		a.SetPermissions(vfs.NewPermissionsFromMode(0644))
	}
	return a, nil
}
