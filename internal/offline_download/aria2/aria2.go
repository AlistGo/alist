package aria2

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/errs"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/aria2/rpc"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var notify = NewNotify()

type Aria2 struct {
	client rpc.Client
}

func (a *Aria2) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (a *Aria2) Name() string {
	return "aria2"
}

func (a *Aria2) Items() []model.SettingItem {
	// aria2 settings
	return []model.SettingItem{
		{Key: conf.Aria2Uri, Value: "http://localhost:6800/jsonrpc", Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
		{Key: conf.Aria2Secret, Value: "", Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
}

func (a *Aria2) Init() (string, error) {
	a.client = nil
	uri := setting.GetStr(conf.Aria2Uri)
	secret := setting.GetStr(conf.Aria2Secret)
	c, err := rpc.New(context.Background(), uri, secret, 4*time.Second, notify)
	if err != nil {
		return "", errors.Wrap(err, "failed to init aria2 client")
	}
	version, err := c.GetVersion()
	if err != nil {
		return "", errors.Wrapf(err, "failed get aria2 version")
	}
	a.client = c
	log.Infof("using aria2 version: %s", version.Version)
	return fmt.Sprintf("aria2 version: %s", version.Version), nil
}

func (a *Aria2) IsReady() bool {
	return a.client != nil
}

func (a *Aria2) AddURL(args *tool.AddUrlArgs) (string, error) {
	options := map[string]interface{}{
		"dir": args.TempDir,
	}
	gid, err := a.client.AddURI([]string{args.Url}, options)
	if err != nil {
		return "", err
	}
	notify.Signals.Store(gid, args.Signal)
	return gid, nil
}

func (a *Aria2) Remove(task *tool.DownloadTask) error {
	_, err := a.client.Remove(task.GID)
	return err
}

func (a *Aria2) Status(task *tool.DownloadTask) (*tool.Status, error) {
	info, err := a.client.TellStatus(task.GID)
	if err != nil {
		return nil, err
	}
	total, err := strconv.ParseInt(info.TotalLength, 10, 64)
	if err != nil {
		total = 0
	}
	downloaded, err := strconv.ParseUint(info.CompletedLength, 10, 64)
	if err != nil {
		downloaded = 0
	}
	s := &tool.Status{
		Completed:  info.Status == "complete",
		Err:        err,
		TotalBytes: total,
	}
	s.Progress = float64(downloaded) / float64(total) * 100
	if len(info.FollowedBy) != 0 {
		s.NewGID = info.FollowedBy[0]
		notify.Signals.Delete(task.GID)
		notify.Signals.Store(s.NewGID, task.Signal)
	}
	switch info.Status {
	case "complete":
		s.Completed = true
	case "error":
		s.Err = errors.Errorf("failed to download %s, error: %s", task.GID, info.ErrorMessage)
	case "active":
		s.Status = "aria2: " + info.Status
		if info.Seeder == "true" {
			s.Completed = true
		}
	case "waiting", "paused":
		s.Status = "aria2: " + info.Status
	case "removed":
		s.Err = errors.Errorf("failed to download %s, removed", task.GID)
	default:
		return nil, errors.Errorf("[aria2] unknown status %s", info.Status)
	}
	return s, nil
}

var _ tool.Tool = (*Aria2)(nil)

func init() {
	tool.Tools.Add(&Aria2{})
}
