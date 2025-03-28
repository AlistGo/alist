package smb

import (
	"errors"
	"io"

	"github.com/KirCute/go-smb2-alist/vfs"
	"github.com/alist-org/alist/v3/internal/errs"
	log "github.com/sirupsen/logrus"
)

type VFSWrapper struct {
	fs   *VFS
	user string
}

func (v *VFSWrapper) GetAttr(handle vfs.VfsHandle) (*vfs.Attributes, error) {
	a, e := v.fs.GetAttr(handle)
	if e != nil {
		log.Errorf("SMB: %s called GetAttr(%d) and got error: %+v", v.user, handle, e)
	}
	return a, e
}

func (v *VFSWrapper) SetAttr(handle vfs.VfsHandle, attributes *vfs.Attributes) (*vfs.Attributes, error) {
	return v.fs.SetAttr(handle, attributes)
}

func (v *VFSWrapper) StatFS(handle vfs.VfsHandle) (*vfs.FSAttributes, error) {
	return v.fs.StatFS(handle)
}

func (v *VFSWrapper) FSync(handle vfs.VfsHandle) error {
	return v.fs.FSync(handle)
}

func (v *VFSWrapper) Flush(handle vfs.VfsHandle) error {
	return v.fs.Flush(handle)
}

func (v *VFSWrapper) Open(s string, i int, i2 int) (vfs.VfsHandle, error) {
	h, e := v.fs.Open(s, i, i2)
	if e == nil {
		log.Infof("SMB: %s called Open(%s, %o) and got result: %d", v.user, s, i, h)
	} else {
		log.Errorf("SMB: %s called Open(%s, %o) and got error: %+v", v.user, s, i, e)
	}
	return h, e
}

func (v *VFSWrapper) Close(handle vfs.VfsHandle) error {
	e := v.fs.Close(handle)
	if e == nil {
		log.Infof("SMB: %s called Close(%d)", v.user, handle)
	} else if errors.Is(e, ErrBadHandle) {
		log.Warnf("SMB: %s called Close(%d) but duplicate", v.user, handle)
	} else {
		log.Errorf("SMB: %s called Close(%d) and got error: %+v", v.user, handle, e)
	}
	return e
}

func (v *VFSWrapper) Lookup(handle vfs.VfsHandle, s string) (*vfs.Attributes, error) {
	a, e := v.fs.Lookup(handle, s)
	if errors.Is(e, errs.ObjectNotFound) {
		log.Warnf("SMB: %s called Lookup(%d, %s) but not found", v.user, handle, s)
	} else if e != nil {
		log.Errorf("SMB: %s called Lookup(%d, %s) and got error: %+v", v.user, handle, s, e)
	}
	return a, e
}

func (v *VFSWrapper) Mkdir(s string, i int) (*vfs.Attributes, error) {
	a, e := v.fs.Mkdir(s, i)
	if e == nil {
		log.Infof("SMB: %s called Mkdir(%s, %d)", v.user, s, i)
	} else {
		log.Errorf("SMB: %s called Mkdir(%s, %d) and got error: %+v", v.user, s, i, e)
	}
	return a, e
}

func (v *VFSWrapper) Read(handle vfs.VfsHandle, bytes []byte, u uint64, i int) (int, error) {
	n, err := v.fs.Read(handle, bytes, u, i)
	if err != nil {
		log.Errorf("SMB: %s called Read(%d, len=%d, offset=%d), read %d bytes and got error %+v", v.user, handle, len(bytes), u, n, err)
	}
	return n, err
}

func (v *VFSWrapper) Write(handle vfs.VfsHandle, bytes []byte, u uint64, i int) (int, error) {
	n, err := v.fs.Write(handle, bytes, u, i)
	if err != nil {
		log.Errorf("SMB: %s called Write(%d, len=%d, offset=%d, mode=%d), write %d bytes and got error %+v", v.user, handle, len(bytes), u, i, n, err)
	}
	return n, err
}

func (v *VFSWrapper) OpenDir(s string) (vfs.VfsHandle, error) {
	h, e := v.fs.OpenDir(s)
	if e == nil {
		log.Infof("SMB: %s called OpenDir(%s) and got result: %d", v.user, s, h)
	} else {
		log.Errorf("SMB: %s called OpenDir(%s) and got error: %+v", v.user, s, e)
	}
	return h, e
}

func (v *VFSWrapper) ReadDir(handle vfs.VfsHandle, i int, i2 int) ([]vfs.DirInfo, error) {
	info, e := v.fs.ReadDir(handle, i, i2)
	if e != nil && !errors.Is(e, io.EOF) {
		log.Errorf("SMB: %s called ReadDir(%d, %d, %d) and got error: %+v", v.user, handle, i, i2, e)
	}
	return info, e
}

func (v *VFSWrapper) Readlink(handle vfs.VfsHandle) (string, error) {
	return v.fs.Readlink(handle)
}

func (v *VFSWrapper) Unlink(handle vfs.VfsHandle) error {
	e := v.fs.Unlink(handle)
	if e == nil {
		log.Infof("SMB: %s called Unlink(%d)", v.user, handle)
	} else {
		log.Errorf("SMB: %s called Unlink(%d) and got error: %+v", v.user, handle, e)
	}
	return e
}

func (v *VFSWrapper) Truncate(handle vfs.VfsHandle, u uint64) error {
	return v.fs.Truncate(handle, u)
}

func (v *VFSWrapper) Rename(handle vfs.VfsHandle, s string, i int) error {
	e := v.fs.Rename(handle, s, i)
	if e == nil {
		log.Infof("SMB: %s called Rename(%d, %s, %d)", v.user, handle, s, i)
	} else {
		log.Errorf("SMB: %s called Rename(%d, %s, %d) and got error: %+v", v.user, handle, s, i, e)
	}
	return e
}

func (v *VFSWrapper) Symlink(handle vfs.VfsHandle, s string, i int) (*vfs.Attributes, error) {
	return v.fs.Symlink(handle, s, i)
}

func (v *VFSWrapper) Link(node vfs.VfsNode, node2 vfs.VfsNode, s string) (*vfs.Attributes, error) {
	return v.fs.Link(node, node2, s)
}

func (v *VFSWrapper) Listxattr(handle vfs.VfsHandle) ([]string, error) {
	return v.fs.Listxattr(handle)
}

func (v *VFSWrapper) Getxattr(handle vfs.VfsHandle, s string, bytes []byte) (int, error) {
	return v.fs.Getxattr(handle, s, bytes)
}

func (v *VFSWrapper) Setxattr(handle vfs.VfsHandle, s string, bytes []byte) error {
	return v.fs.Setxattr(handle, s, bytes)
}

func (v *VFSWrapper) Removexattr(handle vfs.VfsHandle, s string) error {
	return v.fs.Removexattr(handle, s)
}
