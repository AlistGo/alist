package netease_music

import (
	"context"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func (d *NeteaseMusic) request(url, method string, opt ReqOption) ([]byte, error) {
	req := base.RestyClient.R()

	req.SetHeader("Cookie", d.Addition.Cookie)

	if strings.Contains(url, "music.163.com") {
		req.SetHeader("Referer", "https://music.163.com")
	}

	if opt.cookies != nil {
		for _, cookie := range opt.cookies {
			req.SetCookie(cookie)
		}
	}

	if opt.headers != nil {
		for header, value := range opt.headers {
			req.SetHeader(header, value)
		}
	}

	data := opt.data
	if opt.crypto == "weapi" {
		data = weapi(data)
		re, _ := regexp.Compile(`/\w*api/`)
		url = re.ReplaceAllString(url, "/weapi/")
	} else if opt.crypto == "eapi" {
		ch := new(Characteristic).fromDriver(d)
		req.SetCookies(ch.toCookies())
		data = eapi(opt.url, ch.merge(data))
		re, _ := regexp.Compile(`/\w*api/`)
		url = re.ReplaceAllString(url, "/eapi/")
	} else if opt.crypto == "linuxapi" {
		re, _ := regexp.Compile(`/\w*api/`)
		data = linuxapi(map[string]interface{}{
			"url":    re.ReplaceAllString(url, "/api/"),
			"method": method,
			"params": data,
		})
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/60.0.3112.90 Safari/537.36")
		url = "https://music.163.com/api/linux/forward"
	}

	if opt.ctx != nil {
		req.SetContext(opt.ctx)
	}
	if method == http.MethodPost {
		if opt.stream != nil {
			if opt.up == nil {
				opt.up = func(_ float64) {}
			}
			req.SetContentLength(true)
			req.SetBody(driver.NewLimitedUploadStream(opt.ctx, &driver.ReaderUpdatingProgress{
				Reader:         opt.stream,
				UpdateProgress: opt.up,
			}))
		} else {
			req.SetFormData(data)
		}
		res, err := req.Post(url)
		if err != nil {
			return nil, err
		}
		return res.Body(), nil
	}

	if method == http.MethodGet {
		res, err := req.Get(url)
		if err != nil {
			return nil, err
		}
		return res.Body(), nil
	}

	return nil, errs.NotImplement
}

func (d *NeteaseMusic) getSongObjs(args model.ListArgs) ([]model.Obj, error) {
	body, err := d.request("https://music.163.com/weapi/v1/cloud/get", http.MethodPost, ReqOption{
		crypto: "weapi",
		data: map[string]string{
			"limit":  strconv.FormatUint(d.Addition.SongLimit, 10),
			"offset": "0",
		},
		cookies: []*http.Cookie{
			{Name: "os", Value: "pc"},
		},
	})
	if err != nil {
		return nil, err
	}

	var resp ListResp
	err = utils.Json.Unmarshal(body, &resp)
	if err != nil {
		return nil, err
	}

	d.fileMapByName = make(map[string]model.Obj)
	files := make([]model.Obj, 0, len(resp.Data))
	for _, f := range resp.Data {
		song := &model.ObjThumb{
			Object: model.Object{
				IsFolder: false,
				Size:     f.FileSize,
				Name:     f.FileName,
				Modified: time.UnixMilli(f.AddTime),
				ID:       strconv.FormatInt(f.SongId, 10),
			},
			Thumbnail: model.Thumbnail{Thumbnail: f.SimpleSong.Al.PicUrl},
		}
		d.fileMapByName[song.Name] = song
		files = append(files, song)

		// map song id for lyric
		lrcName := strings.Split(f.FileName, ".")[0] + ".lrc"
		lrc := &model.Object{
			IsFolder: false,
			Name:     lrcName,
			Path:     path.Join(args.ReqPath, lrcName),
			ID:       strconv.FormatInt(f.SongId, 10),
		}
		d.fileMapByName[lrc.Name] = lrc
	}

	return files, nil
}

func (d *NeteaseMusic) getSongLink(file model.Obj) (*model.Link, error) {
	body, err := d.request(
		"https://music.163.com/api/song/enhance/player/url", http.MethodPost, ReqOption{
			crypto: "linuxapi",
			data: map[string]string{
				"ids": "[" + file.GetID() + "]",
				"br":  "999000",
			},
			cookies: []*http.Cookie{
				{Name: "os", Value: "pc"},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	var resp SongResp
	err = utils.Json.Unmarshal(body, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) < 1 {
		return nil, errs.ObjectNotFound
	}

	return &model.Link{URL: resp.Data[0].Url}, nil
}

func (d *NeteaseMusic) getLyricObj(file model.Obj) (model.Obj, error) {
	if lrc, ok := file.(*LyricObj); ok {
		return lrc, nil
	}

	body, err := d.request(
		"https://music.163.com/api/song/lyric?_nmclfl=1", http.MethodPost, ReqOption{
			data: map[string]string{
				"id": file.GetID(),
				"tv": "-1",
				"lv": "-1",
				"rv": "-1",
				"kv": "-1",
			},
			cookies: []*http.Cookie{
				{Name: "os", Value: "ios"},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	lyric := utils.Json.Get(body, "lrc", "lyric").ToString()

	return &LyricObj{
		lyric: lyric,
		Object: model.Object{
			IsFolder: false,
			ID:       file.GetID(),
			Name:     file.GetName(),
			Path:     file.GetPath(),
			Size:     int64(len(lyric)),
		},
	}, nil
}

func (d *NeteaseMusic) removeSongObj(file model.Obj) error {
	_, err := d.request("http://music.163.com/weapi/cloud/del", http.MethodPost, ReqOption{
		crypto: "weapi",
		data: map[string]string{
			"songIds": "[" + file.GetID() + "]",
		},
	})

	return err
}

func (d *NeteaseMusic) putSongStream(ctx context.Context, stream model.FileStreamer, up driver.UpdateProgress) error {
	tmp, err := stream.CacheFullInTempFile()
	if err != nil {
		return err
	}

	u := uploader{driver: d, file: tmp}

	err = u.init(stream)
	if err != nil {
		return err
	}

	err = u.checkIfExisted()
	if err != nil {
		return err
	}

	token, err := u.allocToken()
	if err != nil {
		return err
	}

	if u.meta.needUpload {
		err = u.upload(ctx, stream, up)
		if err != nil {
			return err
		}
	}

	err = u.publishInfo(token.resourceId)
	if err != nil {
		return err
	}

	return nil
}
