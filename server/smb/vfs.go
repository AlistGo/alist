package smb

import (
	"context"
	"io"
	"os"
	stdpath "path"
	"sync"

	"github.com/KirCute/go-smb2-alist/vfs"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/xhofe/tache"
)

type VFS struct {
	ctx           context.Context
	openedPath    map[vfs.VfsHandle]*readingFile
	openedTmpFile map[vfs.VfsHandle]*writingFile
	uploadingFile map[string]task.TaskExtensionInfo
	nextHandle    vfs.VfsHandle
	mutex         sync.RWMutex
}

func NewVFS(ctx context.Context) (vfs.VFSFileSystem, error) {
	fs := &VFS{
		ctx:           ctx,
		openedPath:    make(map[vfs.VfsHandle]*readingFile),
		openedTmpFile: make(map[vfs.VfsHandle]*writingFile),
		uploadingFile: make(map[string]task.TaskExtensionInfo),
		nextHandle:    vfs.VfsHandle(1),
		mutex:         sync.RWMutex{},
	}
	root, err := FsGet(ctx, ".")
	if err != nil {
		return nil, err
	}
	fs.openedPath[vfs.VfsHandle(0)] = newRead(".", root)
	user := ctx.Value("user").(*model.User)
	log.Infof("User %s logged in the SMB endpoint", user.Username)
	return &VFSWrapper{fs: fs, user: user.Username}, nil
}

func (fs *VFS) GetAttr(handle vfs.VfsHandle) (*vfs.Attributes, error) {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	if tmp, ok := fs.openedTmpFile[handle]; ok {
		return MakeFileAttribute(tmp.f)
	}
	if r, ok := fs.openedPath[handle]; ok {
		return MakeObjAttribute(r.obj)
	}
	return nil, ErrBadHandle
}

func (fs *VFS) Flush(handle vfs.VfsHandle) error {
	fs.mutex.RLock()
	tmp, ok := fs.openedTmpFile[handle]
	fs.mutex.RUnlock()
	if ok {
		return tmp.f.Sync()
	}
	return nil
}

func (fs *VFS) newUpload(path string) (vfs.VfsHandle, error) {
	f, err := newUpload(fs.ctx, path)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	handle := fs.nextHandle
	fs.nextHandle++
	fs.openedTmpFile[handle] = f
	return handle, nil
}

func (fs *VFS) Open(path string, flags int, _ int) (vfs.VfsHandle, error) {
	if (flags & os.O_APPEND) != 0 {
		return 0, errors.WithStack(errs.NotSupport)
	}
	path = utils.FixAndCleanPath(path)
	fs.mutex.RLock()
	for h, tmp := range fs.openedTmpFile {
		if tmp.path == path {
			fs.mutex.RUnlock()
			return h, nil
		}
	}
	t, ok := fs.uploadingFile[path]
	fs.mutex.RUnlock()
	if ok {
		if t.GetState() == tache.StateSucceeded {
			fs.mutex.Lock()
			delete(fs.uploadingFile, path)
			fs.mutex.Unlock()
		} else if t.GetState() == tache.StateFailed || t.GetState() == tache.StateCanceled {
			fs.mutex.Lock()
			delete(fs.uploadingFile, path)
			fs.mutex.Unlock()
			return 0, errors.WithStack(os.ErrNotExist)
		} else {
			return 0, errors.WithStack(os.ErrPermission)
		}
	}
	obj, err := FsGet(fs.ctx, path)
	if errors.Is(err, errs.ObjectNotFound) {
		if (flags&os.O_RDWR) == 0 || (flags&os.O_CREATE) == 0 {
			return 0, errors.WithStack(os.ErrNotExist)
		}
		return fs.newUpload(path)
	} else if err == nil {
		if (flags&os.O_CREATE) != 0 && (flags&os.O_EXCL) != 0 {
			return 0, errors.WithStack(os.ErrExist)
		}
		if (flags & os.O_RDWR) != 0 {
			if (flags&os.O_CREATE) != 0 && (flags&os.O_TRUNC) != 0 {
				return fs.newUpload(path)
			}
			return 0, errors.WithStack(errs.NotSupport)
		}
		fs.mutex.Lock()
		defer fs.mutex.Unlock()
		handle := fs.nextHandle
		fs.nextHandle++
		fs.openedPath[handle] = newRead(path, obj)
		return handle, nil
	}
	return 0, errors.WithStack(err)
}

func (fs *VFS) Close(handle vfs.VfsHandle) error {
	if handle == 0 {
		return ErrBadHandle
	}
	fs.mutex.RLock()
	tmp, ok := fs.openedTmpFile[handle]
	fs.mutex.RUnlock()
	if ok {
		tsk, err := tmp.close(fs.ctx)
		fs.mutex.Lock()
		delete(fs.openedTmpFile, handle)
		if tsk != nil {
			fs.uploadingFile[tmp.path] = tsk
		}
		fs.mutex.Unlock()
		return errors.WithStack(err)
	}
	fs.mutex.RLock()
	r, ok := fs.openedPath[handle]
	fs.mutex.RUnlock()
	if ok {
		err := r.Close()
		fs.mutex.Lock()
		delete(fs.openedPath, handle)
		fs.mutex.Unlock()
		return errors.WithStack(err)
	}
	return ErrBadHandle
}

func (fs *VFS) Lookup(handle vfs.VfsHandle, name string) (*vfs.Attributes, error) {
	fs.mutex.RLock()
	var p *readingFile
	var ok bool
	if p, ok = fs.openedPath[handle]; !ok {
		fs.mutex.RUnlock()
		return nil, ErrBadHandle
	}
	if name == "/" {
		fs.mutex.RUnlock()
		return MakeObjAttribute(p.obj)
	}
	path := utils.FixAndCleanPath(stdpath.Join(p.path, name))
	for _, tmp := range fs.openedTmpFile {
		if tmp.path == path {
			fs.mutex.RUnlock()
			return MakeFileAttribute(tmp.f)
		}
	}
	t, ok := fs.uploadingFile[path]
	fs.mutex.RUnlock()
	if ok {
		if t.GetState() == tache.StateSucceeded {
			fs.mutex.Lock()
			delete(fs.uploadingFile, path)
			fs.mutex.Unlock()
		} else if t.GetState() == tache.StateFailed || t.GetState() == tache.StateCanceled {
			fs.mutex.Lock()
			delete(fs.uploadingFile, path)
			fs.mutex.Unlock()
			return nil, errors.WithStack(os.ErrNotExist)
		} else {
			return MakeTaskAttribute(t)
		}
	}
	obj, err := FsGet(fs.ctx, path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return MakeObjAttribute(obj)
}

func (fs *VFS) Mkdir(path string, _ int) (*vfs.Attributes, error) {
	obj, err := Mkdir(fs.ctx, path)
	if err != nil {
		return nil, err
	}
	return MakeObjAttribute(obj)
}

func (fs *VFS) Read(handle vfs.VfsHandle, buf []byte, offset uint64, _ int) (n int, err error) {
	fs.mutex.RLock()
	if tmp, ok := fs.openedTmpFile[handle]; ok {
		fs.mutex.RUnlock()
		n, err = tmp.f.ReadAt(buf, int64(offset))
	} else if r, ok := fs.openedPath[handle]; ok {
		fs.mutex.RUnlock()
		err = r.initDownload(fs.ctx)
		if err == nil {
			n, err = r.s.ReadAt(buf, int64(offset))
		}
	} else {
		fs.mutex.RUnlock()
		err = ErrBadHandle
	}
	_ = stream.ClientDownloadLimit.WaitN(fs.ctx, n)
	return
}

func (fs *VFS) Write(handle vfs.VfsHandle, buf []byte, offset uint64, _ int) (n int, err error) {
	fs.mutex.RLock()
	tmp, ok := fs.openedTmpFile[handle]
	fs.mutex.RUnlock()
	if ok {
		n, err = tmp.f.WriteAt(buf, int64(offset))
	} else {
		err = ErrBadHandle
	}
	_ = stream.ClientUploadLimit.WaitN(fs.ctx, n)
	return
}

func (fs *VFS) OpenDir(path string) (vfs.VfsHandle, error) {
	obj, err := FsGet(fs.ctx, path)
	if err != nil {
		return 0, err
	}
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	handle := fs.nextHandle
	fs.nextHandle++
	fs.openedPath[handle] = newRead(path, obj)
	return handle, nil
}

func (fs *VFS) ReadDir(handle vfs.VfsHandle, pos int, _ int) ([]vfs.DirInfo, error) {
	fs.mutex.RLock()
	p, ok := fs.openedPath[handle]
	fs.mutex.RUnlock()
	if !ok {
		return nil, ErrBadHandle
	}
	if pos == 0 && p.dirRead {
		return nil, io.EOF
	}
	p.dirRead = true
	return List(fs.ctx, p.path)
}

func (fs *VFS) Unlink(handle vfs.VfsHandle) error {
	if handle == 0 {
		return ErrBadHandle
	}
	fs.mutex.RLock()
	r, ok := fs.openedPath[handle]
	fs.mutex.RUnlock()
	if ok {
		_ = r.Close()
		fs.mutex.Lock()
		delete(fs.openedPath, handle)
		fs.mutex.Unlock()
		return Remove(fs.ctx, r.path)
	}
	return ErrBadHandle
}

func (fs *VFS) Rename(handle vfs.VfsHandle, to string, _ int) error {
	if handle == 0 {
		return ErrBadHandle
	}
	fs.mutex.RLock()
	r, ok := fs.openedPath[handle]
	fs.mutex.RUnlock()
	if ok {
		_ = r.Close()
		fs.mutex.Lock()
		delete(fs.openedPath, handle)
		fs.mutex.Unlock()
		return Rename(fs.ctx, r.path, to)
	}
	return ErrBadHandle
}

func (fs *VFS) StatFS(vfs.VfsHandle) (*vfs.FSAttributes, error) {
	return &vfs.FSAttributes{}, nil
}

func (fs *VFS) SetAttr(vfs.VfsHandle, *vfs.Attributes) (*vfs.Attributes, error) {
	return nil, nil
}

func (fs *VFS) FSync(vfs.VfsHandle) error {
	return nil
}

func (fs *VFS) Readlink(vfs.VfsHandle) (string, error) {
	return "", errs.NotSupport
}

func (fs *VFS) Truncate(vfs.VfsHandle, uint64) error {
	return errs.NotSupport
}

func (fs *VFS) Symlink(vfs.VfsHandle, string, int) (*vfs.Attributes, error) {
	return nil, errs.NotSupport
}

func (fs *VFS) Link(vfs.VfsNode, vfs.VfsNode, string) (*vfs.Attributes, error) {
	return nil, nil
}

func (fs *VFS) Listxattr(vfs.VfsHandle) ([]string, error) {
	return []string{}, nil
}

func (fs *VFS) Getxattr(vfs.VfsHandle, string, []byte) (int, error) {
	return 0, errs.NotSupport
}

func (fs *VFS) Setxattr(vfs.VfsHandle, string, []byte) error {
	return errs.NotSupport
}

func (fs *VFS) Removexattr(vfs.VfsHandle, string) error {
	return errs.NotSupport
}
