package netease_music

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/alist-org/alist/v3/server/common"
)

type HostsResp struct {
	Upload []string `json:"upload"`
}

type SongResp struct {
	Data []struct {
		Url string `json:"url"`
	} `json:"data"`
}

type ListResp struct {
	Size    int64 `json:"size"`
	MaxSize int64 `json:"maxSize"`
	Data    []struct {
		AddTime    int64  `json:"addTime"`
		FileName   string `json:"fileName"`
		FileSize   int64  `json:"fileSize"`
		SongId     int64  `json:"songId"`
		SimpleSong struct {
			Al struct {
				PicUrl string `json:"picUrl"`
			} `json:"al"`
		} `json:"simpleSong"`
	} `json:"data"`
}

type LyricObj struct {
	model.Object
	lyric string
}

func (lrc *LyricObj) getProxyLink(args model.LinkArgs) *model.Link {
	rawURL := common.GetApiUrl(args.HttpReq) + "/p" + lrc.Path
	rawURL = utils.EncodePath(rawURL, true) + "?type=parsed&sign=" + sign.Sign(lrc.Path)
	return &model.Link{URL: rawURL}
}

func (lrc *LyricObj) getLyricLink() *model.Link {
	reader := strings.NewReader(lrc.lyric)
	return &model.Link{
		RangeReadCloser: &model.RangeReadCloser{
			RangeReader: func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
				if httpRange.Length < 0 {
					return io.NopCloser(reader), nil
				}
				sr := io.NewSectionReader(reader, httpRange.Start, httpRange.Length)
				return io.NopCloser(sr), nil
			},
		},
	}
}

type ReqOption struct {
	crypto  string
	stream  model.FileStreamer
	up      driver.UpdateProgress
	ctx     context.Context
	data    map[string]string
	headers map[string]string
	cookies []*http.Cookie
	url     string
}

type Characteristic map[string]string

func (ch *Characteristic) fromDriver(d *NeteaseMusic) *Characteristic {
	*ch = map[string]string{
		"osver":       "",
		"deviceId":    "",
		"mobilename":  "",
		"appver":      "6.1.1",
		"versioncode": "140",
		"buildver":    strconv.FormatInt(time.Now().Unix(), 10),
		"resolution":  "1920x1080",
		"os":          "android",
		"channel":     "",
		"requestId":   strconv.FormatInt(time.Now().Unix()*1000, 10) + strconv.Itoa(int(random.RangeInt64(0, 1000))),
		"MUSIC_U":     d.musicU,
	}
	return ch
}

func (ch Characteristic) toCookies() []*http.Cookie {
	cookies := make([]*http.Cookie, 0)
	for k, v := range ch {
		cookies = append(cookies, &http.Cookie{Name: k, Value: v})
	}
	return cookies
}

func (ch *Characteristic) merge(data map[string]string) map[string]interface{} {
	body := map[string]interface{}{
		"header": ch,
	}
	for k, v := range data {
		body[k] = v
	}
	return body
}
