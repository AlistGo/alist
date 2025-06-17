package server

import (
	"context"
	"github.com/KirCute/sftpd-alist"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/ftp"
	"github.com/alist-org/alist/v3/server/sftp"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
	"net/http"
	"time"
)

type SftpDriver struct {
	proxyHeader *http.Header
	config      *sftpd.Config
}

func NewSftpDriver() (*SftpDriver, error) {
	sftp.InitHostKey()
	header := &http.Header{}
	header.Add("User-Agent", setting.GetStr(conf.FTPProxyUserAgent))
	return &SftpDriver{
		proxyHeader: header,
	}, nil
}

func (d *SftpDriver) GetConfig() *sftpd.Config {
	if d.config != nil {
		return d.config
	}
	serverConfig := ssh.ServerConfig{
		NoClientAuth:         true,
		NoClientAuthCallback: d.NoClientAuth,
		PasswordCallback:     d.PasswordAuth,
		PublicKeyCallback:    d.PublicKeyAuth,
		AuthLogCallback:      d.AuthLogCallback,
		BannerCallback:       d.GetBanner,
	}
	for _, k := range sftp.SSHSigners {
		serverConfig.AddHostKey(k)
	}
	d.config = &sftpd.Config{
		ServerConfig: serverConfig,
		HostPort:     conf.Conf.SFTP.Listen,
		ErrorLogFunc: utils.Log.Error,
		//DebugLogFunc: utils.Log.Debugf,
	}
	return d.config
}

func (d *SftpDriver) GetFileSystem(sc *ssh.ServerConn) (sftpd.FileSystem, error) {
	userObj, err := op.GetUserByName(sc.User())
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "user", userObj)
	ctx = context.WithValue(ctx, "meta_pass", "")
	ctx = context.WithValue(ctx, "client_ip", sc.RemoteAddr().String())
	ctx = context.WithValue(ctx, "proxy_header", d.proxyHeader)
	return &sftp.DriverAdapter{FtpDriver: ftp.NewAferoAdapter(ctx)}, nil
}

func (d *SftpDriver) Close() {
}

func (d *SftpDriver) NoClientAuth(conn ssh.ConnMetadata) (*ssh.Permissions, error) {
	if conn.User() != "guest" {
		return nil, errors.New("only guest is allowed to login without authorization")
	}
	guest, err := op.GetGuest()
	if err != nil {
		return nil, err
	}
	if guest.Disabled || !guest.CanFTPAccess() {
		return nil, errors.New("user is not allowed to access via SFTP")
	}
	return nil, nil
}

func (d *SftpDriver) PasswordAuth(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	userObj, err := op.GetUserByName(conn.User())
	if err != nil {
		return nil, err
	}
	if userObj.Disabled || !userObj.CanFTPAccess() {
		return nil, errors.New("user is not allowed to access via SFTP")
	}
	passHash := model.StaticHash(string(password))
	if err = userObj.ValidatePwdStaticHash(passHash); err != nil {
		return nil, err
	}
	return nil, nil
}

func (d *SftpDriver) PublicKeyAuth(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	userObj, err := op.GetUserByName(conn.User())
	if err != nil {
		return nil, err
	}
	if userObj.Disabled || !userObj.CanFTPAccess() {
		return nil, errors.New("user is not allowed to access via SFTP")
	}
	keys, _, err := op.GetSSHPublicKeyByUserId(userObj.ID, 1, -1)
	if err != nil {
		return nil, err
	}
	marshal := string(key.Marshal())
	for _, sk := range keys {
		if marshal != sk.KeyStr {
			pubKey, _, _, _, e := ssh.ParseAuthorizedKey([]byte(sk.KeyStr))
			if e != nil || marshal != string(pubKey.Marshal()) {
				continue
			}
		}
		sk.LastUsedTime = time.Now()
		_ = op.UpdateSSHPublicKey(&sk)
		return nil, nil
	}
	return nil, errors.New("public key refused")
}

func (d *SftpDriver) AuthLogCallback(conn ssh.ConnMetadata, method string, err error) {
	ip := conn.RemoteAddr().String()
	if err == nil {
		utils.Log.Infof("[SFTP] %s(%s) logged in via %s", conn.User(), ip, method)
	} else if method != "none" {
		utils.Log.Infof("[SFTP] %s(%s) tries logging in via %s but with error: %s", conn.User(), ip, method, err)
	}
}

func (d *SftpDriver) GetBanner(_ ssh.ConnMetadata) string {
	return setting.GetStr(conf.Announcement)
}
