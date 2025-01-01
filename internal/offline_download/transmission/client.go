package transmission

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/hekmon/transmissionrpc/v3"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Transmission struct {
	client *transmissionrpc.Client
}

func (t *Transmission) Run(task *tool.DownloadTask) error {
	return errs.NotSupport
}

func (t *Transmission) Name() string {
	return "Transmission"
}

func (t *Transmission) Items() []model.SettingItem {
	// transmission settings
	return []model.SettingItem{
		{Key: conf.TransmissionUri, Value: "http://localhost:9091/transmission/rpc", Type: conf.TypeString, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
		{Key: conf.TransmissionSeedtime, Value: "0", Type: conf.TypeNumber, Group: model.OFFLINE_DOWNLOAD, Flag: model.PRIVATE},
	}
}

func (t *Transmission) Init() (string, error) {
	t.client = nil
	uri := setting.GetStr(conf.TransmissionUri)
	endpoint, err := url.Parse(uri)
	if err != nil {
		return "", errors.Wrap(err, "failed to init transmission client")
	}
	c, err := transmissionrpc.New(endpoint, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to init transmission client")
	}

	ok, serverVersion, serverMinimumVersion, err := c.RPCVersion(context.Background())
	if err != nil {
		return "", errors.Wrapf(err, "failed get transmission version")
	}

	if !ok {
		return "", fmt.Errorf("remote transmission RPC version (v%d) is incompatible with the transmission library (v%d): remote needs at least v%d",
			serverVersion, transmissionrpc.RPCVersion, serverMinimumVersion)
	}

	t.client = c
	log.Infof("remote transmission RPC version (v%d) is compatible with our transmissionrpc library (v%d)\n",
		serverVersion, transmissionrpc.RPCVersion)
	log.Infof("using transmission version: %d", serverVersion)
	return fmt.Sprintf("transmission version: %d", serverVersion), nil
}

func (t *Transmission) IsReady() bool {
	return t.client != nil
}

func (t *Transmission) AddURL(args *tool.AddUrlArgs) (string, error) {
	endpoint, err := url.Parse(args.Url)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse transmission uri")
	}

	rpcPayload := transmissionrpc.TorrentAddPayload{
		DownloadDir: &args.TempDir,
	}
	// http url for .torrent file
	if endpoint.Scheme == "http" || endpoint.Scheme == "https" {
		resp, err := http.Get(args.Url)
		if err != nil {
			return "", errors.Wrap(err, "failed to get .torrent file")
		}
		defer resp.Body.Close()
		buffer := new(bytes.Buffer)
		encoder := base64.NewEncoder(base64.StdEncoding, buffer)
		// Stream file to the encoder
		if _, err = io.Copy(encoder, resp.Body); err != nil {
			return "", errors.Wrap(err, "can't copy file content into the base64 encoder")
		}
		// Flush last bytes
		if err = encoder.Close(); err != nil {
			return "", errors.Wrap(err, "can't flush last bytes of the base64 encoder")
		}
		// Get the string form
		b64 := buffer.String()
		rpcPayload.MetaInfo = &b64
	} else { // magnet uri
		rpcPayload.Filename = &args.Url
	}

	torrent, err := t.client.TorrentAdd(context.TODO(), rpcPayload)
	if err != nil {
		return "", err
	}

	if torrent.ID == nil {
		return "", fmt.Errorf("failed get torrent ID")
	}
	gid := strconv.FormatInt(*torrent.ID, 10)
	return gid, nil
}

func (t *Transmission) Remove(task *tool.DownloadTask) error {
	gid, err := strconv.ParseInt(task.GID, 10, 64)
	if err != nil {
		return err
	}
	err = t.client.TorrentRemove(context.TODO(), transmissionrpc.TorrentRemovePayload{
		IDs:             []int64{gid},
		DeleteLocalData: false,
	})
	return err
}

func (t *Transmission) Status(task *tool.DownloadTask) (*tool.Status, error) {
	gid, err := strconv.ParseInt(task.GID, 10, 64)
	if err != nil {
		return nil, err
	}
	infos, err := t.client.TorrentGetAllFor(context.TODO(), []int64{gid})
	if err != nil {
		return nil, err
	}

	if len(infos) < 1 {
		return nil, fmt.Errorf("failed get status, wrong gid: %s", task.GID)
	}
	info := infos[0]

	s := &tool.Status{
		Completed: *info.IsFinished,
		Err:       err,
	}
	s.Progress = *info.PercentDone * 100
	s.TotalBytes = int64(*info.SizeWhenDone / 8)

	switch *info.Status {
	case transmissionrpc.TorrentStatusCheckWait,
		transmissionrpc.TorrentStatusDownloadWait,
		transmissionrpc.TorrentStatusCheck,
		transmissionrpc.TorrentStatusDownload,
		transmissionrpc.TorrentStatusIsolated:
		s.Status = "[transmission] " + info.Status.String()
	case transmissionrpc.TorrentStatusSeedWait,
		transmissionrpc.TorrentStatusSeed:
		s.Completed = true
	case transmissionrpc.TorrentStatusStopped:
		s.Err = errors.Errorf("[transmission] failed to download %s, status: %s, error: %s", task.GID, info.Status.String(), *info.ErrorString)
	default:
		s.Err = errors.Errorf("[transmission] unknown status occurred downloading %s, err: %s", task.GID, *info.ErrorString)
	}
	return s, nil
}

var _ tool.Tool = (*Transmission)(nil)

func init() {
	tool.Tools.Add(&Transmission{})
}
