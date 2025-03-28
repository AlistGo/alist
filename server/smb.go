package server

import (
	"context"
	"github.com/alist-org/alist/v3/internal/errs"

	smb2 "github.com/KirCute/go-smb2-alist/server"
	"github.com/KirCute/go-smb2-alist/vfs"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/smb"
)

func NewSmbServer() (*smb2.Server, error) {
	srv := smb2.NewServer(
		&smb2.ServerConfig{
			MaxIOReads:  conf.Conf.SMB.MaxIOReads,
			MaxIOWrites: conf.Conf.SMB.MaxIOWrites,
			Xatrrs:      false,
		},
		&smb2.NTLMAuthenticator{
			TargetSPN:    conf.Conf.SMB.TargetSPN,
			NbDomain:     conf.Conf.SMB.NbDomain,
			NbName:       conf.Conf.SMB.NbName,
			DnsName:      conf.Conf.SMB.DnsName,
			DnsDomain:    conf.Conf.SMB.DnsDomain,
			UserPassword: GetUserPassword,
			AllowGuest:   AllowGuest,
		},
		GetUserFileSystem,
	)
	return srv, nil
}

func AllowGuest() bool {
	guest, err := op.GetGuest()
	return err == nil && !guest.Disabled
}

func GetUserPassword(user string) (string, bool) {
	u, err := op.GetUserByName(user)
	if err != nil {
		return "", false
	}
	if u.IsGuest() || !u.CanSMBAccess() {
		return "", false
	}
	return u.PwdHash[:16], true
}

func GetUserFileSystem(user string) (map[string]vfs.VFSFileSystem, error) {
	var userObj *model.User
	var err error
	if user == "" {
		userObj, err = op.GetGuest()
	} else {
		userObj, err = op.GetUserByName(user)
	}
	if err != nil {
		return nil, err
	}
	if !userObj.CanSMBAccess() { // For allow guest case
		return nil, errs.PermissionDenied
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "user", userObj)
	fs, err := smb.NewVFS(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]vfs.VFSFileSystem{
		conf.Conf.SMB.ShareName: fs,
	}, nil
}
