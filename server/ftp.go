package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	ftpserver "github.com/KirCute/ftpserverlib-pasvportmap"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/ftp"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type FtpMainDriver struct {
	settings     *ftpserver.Settings
	proxyHeader  *http.Header
	clients      map[uint32]ftpserver.ClientContext
	shutdownLock sync.RWMutex
	isShutdown   bool
	tlsConfig    *tls.Config
}

func NewMainDriver() (*FtpMainDriver, error) {
	header := &http.Header{}
	header.Add("User-Agent", setting.GetStr(conf.FTPProxyUserAgent))
	transferType := ftpserver.TransferTypeASCII
	if conf.Conf.FTP.DefaultTransferBinary {
		transferType = ftpserver.TransferTypeBinary
	}
	activeConnCheck := ftpserver.IPMatchDisabled
	if conf.Conf.FTP.EnableActiveConnIPCheck {
		activeConnCheck = ftpserver.IPMatchRequired
	}
	pasvConnCheck := ftpserver.IPMatchDisabled
	if conf.Conf.FTP.EnablePasvConnIPCheck {
		pasvConnCheck = ftpserver.IPMatchRequired
	}
	tlsRequired := ftpserver.ClearOrEncrypted
	if setting.GetBool(conf.FTPImplicitTLS) {
		tlsRequired = ftpserver.ImplicitEncryption
	} else if setting.GetBool(conf.FTPMandatoryTLS) {
		tlsRequired = ftpserver.MandatoryEncryption
	}
	tlsConf, err := getTlsConf(setting.GetStr(conf.FTPTLSPrivateKeyPath), setting.GetStr(conf.FTPTLSPublicCertPath))
	if err != nil && tlsRequired != ftpserver.ClearOrEncrypted {
		return nil, fmt.Errorf("FTP mandatory TLS has been enabled, but the certificate failed to load: %w", err)
	}
	return &FtpMainDriver{
		settings: &ftpserver.Settings{
			ListenAddr:                conf.Conf.FTP.Listen,
			PublicHost:                lookupIP(setting.GetStr(conf.FTPPublicHost)),
			PassiveTransferPortGetter: newPortMapper(setting.GetStr(conf.FTPPasvPortMap)),
			FindPasvPortAttempts:      conf.Conf.FTP.FindPasvPortAttempts,
			ActiveTransferPortNon20:   conf.Conf.FTP.ActiveTransferPortNon20,
			IdleTimeout:               conf.Conf.FTP.IdleTimeout,
			ConnectionTimeout:         conf.Conf.FTP.ConnectionTimeout,
			DisableMLSD:               false,
			DisableMLST:               false,
			DisableMFMT:               true,
			Banner:                    setting.GetStr(conf.Announcement),
			TLSRequired:               tlsRequired,
			DisableLISTArgs:           false,
			DisableSite:               false,
			DisableActiveMode:         conf.Conf.FTP.DisableActiveMode,
			EnableHASH:                false,
			DisableSTAT:               false,
			DisableSYST:               false,
			EnableCOMB:                false,
			DefaultTransferType:       transferType,
			ActiveConnectionsCheck:    activeConnCheck,
			PasvConnectionsCheck:      pasvConnCheck,
			SiteHandlers: map[string]ftpserver.SiteHandler{
				"SIZE": ftp.HandleSIZE,
			},
		},
		proxyHeader:  header,
		clients:      make(map[uint32]ftpserver.ClientContext),
		shutdownLock: sync.RWMutex{},
		isShutdown:   false,
		tlsConfig:    tlsConf,
	}, nil
}

func (d *FtpMainDriver) GetSettings() (*ftpserver.Settings, error) {
	return d.settings, nil
}

func (d *FtpMainDriver) ClientConnected(cc ftpserver.ClientContext) (string, error) {
	if d.isShutdown || !d.shutdownLock.TryRLock() {
		return "", errors.New("server has shutdown")
	}
	defer d.shutdownLock.RUnlock()
	d.clients[cc.ID()] = cc
	return "AList FTP Endpoint", nil
}

func (d *FtpMainDriver) ClientDisconnected(cc ftpserver.ClientContext) {
	err := cc.Close()
	if err != nil {
		utils.Log.Errorf("failed to close client: %v", err)
	}
	delete(d.clients, cc.ID())
}

func (d *FtpMainDriver) AuthUser(cc ftpserver.ClientContext, user, pass string) (ftpserver.ClientDriver, error) {
	var userObj *model.User
	var err error
	if user == "anonymous" || user == "guest" {
		userObj, err = op.GetGuest()
		if err != nil {
			return nil, err
		}
	} else {
		userObj, err = op.GetUserByName(user)
		if err != nil {
			return nil, err
		}
		passHash := model.StaticHash(pass)
		if err = userObj.ValidatePwdStaticHash(passHash); err != nil {
			return nil, err
		}
	}
	if userObj.Disabled || !userObj.CanFTPAccess() {
		return nil, errors.New("user is not allowed to access via FTP")
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "user", userObj)
	if user == "anonymous" || user == "guest" {
		ctx = context.WithValue(ctx, "meta_pass", pass)
	} else {
		ctx = context.WithValue(ctx, "meta_pass", "")
	}
	ctx = context.WithValue(ctx, "client_ip", cc.RemoteAddr().String())
	ctx = context.WithValue(ctx, "proxy_header", d.proxyHeader)
	return ftp.NewAferoAdapter(ctx), nil
}

func (d *FtpMainDriver) GetTLSConfig() (*tls.Config, error) {
	if d.tlsConfig == nil {
		return nil, errors.New("TLS config not provided")
	}
	return d.tlsConfig, nil
}

func (d *FtpMainDriver) Stop() {
	d.isShutdown = true
	d.shutdownLock.Lock()
	defer d.shutdownLock.Unlock()
	for _, value := range d.clients {
		_ = value.Close()
	}
}

func lookupIP(host string) string {
	if host == "" || net.ParseIP(host) != nil {
		return host
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		utils.Log.Fatalf("given FTP public host is invalid, and the default value will be used: %v", err)
		return ""
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String()
		}
	}
	v6 := ips[0].String()
	utils.Log.Warnf("no IPv4 record looked up, %s will be used as public host, and it might do not work.", v6)
	return v6
}

func newPortMapper(str string) ftpserver.PasvPortGetter {
	if str == "" {
		return nil
	}
	pasvPortMappers := strings.Split(strings.Replace(str, "\n", ",", -1), ",")
	type group struct {
		ExposedStart  int
		ListenedStart int
		Length        int
	}
	groups := make([]group, len(pasvPortMappers))
	totalLength := 0
	convertToPorts := func(str string) (int, int, error) {
		start, end, multi := strings.Cut(str, "-")
		if multi {
			si, err := strconv.Atoi(start)
			if err != nil {
				return 0, 0, err
			}
			ei, err := strconv.Atoi(end)
			if err != nil {
				return 0, 0, err
			}
			if ei < si || ei < 1024 || si < 1024 || ei > 65535 || si > 65535 {
				return 0, 0, errors.New("invalid port")
			}
			return si, ei - si + 1, nil
		} else {
			ret, err := strconv.Atoi(str)
			if err != nil {
				return 0, 0, err
			} else {
				return ret, 1, nil
			}
		}
	}
	for i, mapper := range pasvPortMappers {
		var err error
		exposed, listened, mapped := strings.Cut(mapper, ":")
		for {
			if mapped {
				var es, ls, el, ll int
				es, el, err = convertToPorts(exposed)
				if err != nil {
					break
				}
				ls, ll, err = convertToPorts(listened)
				if err != nil {
					break
				}
				if el != ll {
					err = errors.New("the number of exposed ports and listened ports does not match")
					break
				}
				groups[i].ExposedStart = es
				groups[i].ListenedStart = ls
				groups[i].Length = el
				totalLength += el
			} else {
				var start, length int
				start, length, err = convertToPorts(mapper)
				groups[i].ExposedStart = start
				groups[i].ListenedStart = start
				groups[i].Length = length
				totalLength += length
			}
			break
		}
		if err != nil {
			utils.Log.Fatalf("failed to convert FTP PASV port mapper %s: %v, the port mapper will be ignored.", mapper, err)
			return nil
		}
	}
	return func() (int, int, bool) {
		idxPort := rand.Intn(totalLength)
		for _, g := range groups {
			if idxPort >= g.Length {
				idxPort -= g.Length
			} else {
				return g.ExposedStart + idxPort, g.ListenedStart + idxPort, true
			}
		}
		// unreachable
		return 0, 0, false
	}
}

func getTlsConf(keyPath, certPath string) (*tls.Config, error) {
	if keyPath == "" || certPath == "" {
		return nil, errors.New("private key or certificate is not provided")
	}
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}
