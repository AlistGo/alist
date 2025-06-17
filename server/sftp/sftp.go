package sftp

import (
	"github.com/KirCute/sftpd-alist"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/ftp"
	"os"
)

type DriverAdapter struct {
	FtpDriver *ftp.AferoAdapter
}

func (s *DriverAdapter) OpenFile(_ string, _ uint32, _ *sftpd.Attr) (sftpd.File, error) {
	// See also GetHandle
	return nil, errs.NotImplement
}

func (s *DriverAdapter) OpenDir(_ string) (sftpd.Dir, error) {
	// See also GetHandle
	return nil, errs.NotImplement
}

func (s *DriverAdapter) Remove(name string) error {
	return s.FtpDriver.Remove(name)
}

func (s *DriverAdapter) Rename(old, new string, _ uint32) error {
	return s.FtpDriver.Rename(old, new)
}

func (s *DriverAdapter) Mkdir(name string, attr *sftpd.Attr) error {
	return s.FtpDriver.Mkdir(name, attr.Mode)
}

func (s *DriverAdapter) Rmdir(name string) error {
	return s.Remove(name)
}

func (s *DriverAdapter) Stat(name string, _ bool) (*sftpd.Attr, error) {
	stat, err := s.FtpDriver.Stat(name)
	if err != nil {
		return nil, err
	}
	return fileInfoToSftpAttr(stat), nil
}

func (s *DriverAdapter) SetStat(_ string, _ *sftpd.Attr) error {
	return errs.NotSupport
}

func (s *DriverAdapter) ReadLink(_ string) (string, error) {
	return "", errs.NotSupport
}

func (s *DriverAdapter) CreateLink(_, _ string, _ uint32) error {
	return errs.NotSupport
}

func (s *DriverAdapter) RealPath(path string) (string, error) {
	return utils.FixAndCleanPath(path), nil
}

func (s *DriverAdapter) GetHandle(name string, flags uint32, _ *sftpd.Attr, offset uint64) (sftpd.FileTransfer, error) {
	return s.FtpDriver.GetHandle(name, sftpFlagToOpenMode(flags), int64(offset))
}

func (s *DriverAdapter) ReadDir(name string) ([]sftpd.NamedAttr, error) {
	dir, err := s.FtpDriver.ReadDir(name)
	if err != nil {
		return nil, err
	}
	ret := make([]sftpd.NamedAttr, len(dir))
	for i, d := range dir {
		ret[i] = *fileInfoToSftpNamedAttr(d)
	}
	return ret, nil
}

// From leffss/sftpd
func sftpFlagToOpenMode(flags uint32) int {
	mode := 0
	if (flags & SSH_FXF_READ) != 0 {
		mode |= os.O_RDONLY
	}
	if (flags & SSH_FXF_WRITE) != 0 {
		mode |= os.O_WRONLY
	}
	if (flags & SSH_FXF_APPEND) != 0 {
		mode |= os.O_APPEND
	}
	if (flags & SSH_FXF_CREAT) != 0 {
		mode |= os.O_CREATE
	}
	if (flags & SSH_FXF_TRUNC) != 0 {
		mode |= os.O_TRUNC
	}
	if (flags & SSH_FXF_EXCL) != 0 {
		mode |= os.O_EXCL
	}
	return mode
}

func fileInfoToSftpAttr(stat os.FileInfo) *sftpd.Attr {
	ret := &sftpd.Attr{}
	ret.Flags |= sftpd.ATTR_SIZE
	ret.Size = uint64(stat.Size())
	ret.Flags |= sftpd.ATTR_MODE
	ret.Mode = stat.Mode()
	ret.Flags |= sftpd.ATTR_TIME
	ret.ATime = stat.Sys().(model.Obj).CreateTime()
	ret.MTime = stat.ModTime()
	return ret
}

func fileInfoToSftpNamedAttr(stat os.FileInfo) *sftpd.NamedAttr {
	return &sftpd.NamedAttr{
		Name: stat.Name(),
		Attr: *fileInfoToSftpAttr(stat),
	}
}
