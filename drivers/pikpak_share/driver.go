package pikpak_share

import (
	"context"
	"github.com/alist-org/alist/v3/internal/op"
	"net/http"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type PikPakShare struct {
	model.Storage
	Addition
	*Common
	PassCodeToken string
}

func (d *PikPakShare) Config() driver.Config {
	return config
}

func (d *PikPakShare) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *PikPakShare) Init(ctx context.Context) error {
	if d.Common == nil {
		d.Common = &Common{
			DeviceID:  utils.GetMD5EncodeStr(d.Addition.ShareId + d.Addition.SharePwd + time.Now().String()),
			UserAgent: "",
			RefreshCTokenCk: func(token string) {
				d.Common.CaptchaToken = token
				op.MustSaveDriverStorage(d)
			},
		}
	}

	if d.Addition.DeviceID != "" {
		d.SetDeviceID(d.Addition.DeviceID)
	} else {
		d.Addition.DeviceID = d.Common.DeviceID
		op.MustSaveDriverStorage(d)
	}

	if d.Platform == "android" {
		d.ClientID = AndroidClientID
		d.ClientSecret = AndroidClientSecret
		d.ClientVersion = AndroidClientVersion
		d.PackageName = AndroidPackageName
		d.Algorithms = AndroidAlgorithms
		d.UserAgent = BuildCustomUserAgent(d.GetDeviceID(), AndroidClientID, AndroidPackageName, AndroidSdkVersion, AndroidClientVersion, AndroidPackageName, "")
	} else if d.Platform == "web" {
		d.ClientID = WebClientID
		d.ClientSecret = WebClientSecret
		d.ClientVersion = WebClientVersion
		d.PackageName = WebPackageName
		d.Algorithms = WebAlgorithms
		d.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"
	} else if d.Platform == "pc" {
		d.ClientID = PCClientID
		d.ClientSecret = PCClientSecret
		d.ClientVersion = PCClientVersion
		d.PackageName = PCPackageName
		d.Algorithms = PCAlgorithms
		d.UserAgent = "MainWindow Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) PikPak/2.6.11.4955 Chrome/100.0.4896.160 Electron/18.3.15 Safari/537.36"
	}

	// 获取CaptchaToken
	err := d.RefreshCaptchaToken(GetAction(http.MethodGet, "https://api-drive.mypikpak.net/drive/v1/share:batch_file_info"), "")
	if err != nil {
		return err
	}

	if d.SharePwd != "" {
		return d.getSharePassToken()
	}

	return nil
}

func (d *PikPakShare) Drop(ctx context.Context) error {
	return nil
}

func (d *PikPakShare) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *PikPakShare) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var resp ShareResp
	query := map[string]string{
		"share_id":        d.ShareId,
		"file_id":         file.GetID(),
		"pass_code_token": d.PassCodeToken,
	}
	_, err := d.request("https://api-drive.mypikpak.net/drive/v1/share/file_info", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(query)
	}, &resp)
	if err != nil {
		return nil, err
	}

	downloadUrl := resp.FileInfo.WebContentLink
	if downloadUrl == "" && len(resp.FileInfo.Medias) > 0 {
		// 使用转码后的链接
		if d.Addition.UseTransCodingAddress && len(resp.FileInfo.Medias) > 1 {
			downloadUrl = resp.FileInfo.Medias[1].Link.Url
		} else {
			downloadUrl = resp.FileInfo.Medias[0].Link.Url
		}

	}

	return &model.Link{
		URL: downloadUrl,
	}, nil
}

var _ driver.Driver = (*PikPakShare)(nil)
