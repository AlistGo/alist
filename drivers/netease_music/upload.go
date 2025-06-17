package netease_music

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"github.com/alist-org/alist/v3/internal/driver"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/dhowden/tag"
)

type token struct {
	resourceId string
	objectKey  string
	token      string
}

type songmeta struct {
	needUpload bool
	songId     string
	name       string
	artist     string
	album      string
}

type uploader struct {
	driver   *NeteaseMusic
	file     model.File
	meta     songmeta
	md5      string
	ext      string
	size     string
	filename string
}

func (u *uploader) init(stream model.FileStreamer) error {
	u.filename = stream.GetName()
	u.size = strconv.FormatInt(stream.GetSize(), 10)

	u.ext = "mp3"
	if strings.HasSuffix(stream.GetMimetype(), "flac") {
		u.ext = "flac"
	}

	h := md5.New()
	_, err := utils.CopyWithBuffer(h, stream)
	if err != nil {
		return err
	}
	u.md5 = hex.EncodeToString(h.Sum(nil))
	_, err = u.file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	if m, err := tag.ReadFrom(u.file); err != nil {
		u.meta = songmeta{}
	} else {
		u.meta = songmeta{
			name:   m.Title(),
			artist: m.Artist(),
			album:  m.Album(),
		}
	}
	if u.meta.name == "" {
		u.meta.name = u.filename
	}
	if u.meta.album == "" {
		u.meta.album = "未知专辑"
	}
	if u.meta.artist == "" {
		u.meta.artist = "未知艺术家"
	}
	_, err = u.file.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return nil
}

func (u *uploader) checkIfExisted() error {
	body, err := u.driver.request("https://interface.music.163.com/api/cloud/upload/check", http.MethodPost,
		ReqOption{
			crypto: "weapi",
			data: map[string]string{
				"ext":     "",
				"songId":  "0",
				"version": "1",
				"bitrate": "999000",
				"length":  u.size,
				"md5":     u.md5,
			},
			cookies: []*http.Cookie{
				{Name: "os", Value: "pc"},
				{Name: "appver", Value: "2.9.7"},
			},
		},
	)
	if err != nil {
		return err
	}

	u.meta.songId = utils.Json.Get(body, "songId").ToString()
	u.meta.needUpload = utils.Json.Get(body, "needUpload").ToBool()

	return nil
}

func (u *uploader) allocToken(bucket ...string) (token, error) {
	if len(bucket) == 0 {
		bucket = []string{""}
	}

	body, err := u.driver.request("https://music.163.com/weapi/nos/token/alloc", http.MethodPost, ReqOption{
		crypto: "weapi",
		data: map[string]string{
			"bucket":      bucket[0],
			"local":       "false",
			"type":        "audio",
			"nos_product": "3",
			"filename":    u.filename,
			"md5":         u.md5,
			"ext":         u.ext,
		},
	})
	if err != nil {
		return token{}, err
	}

	return token{
		resourceId: utils.Json.Get(body, "result", "resourceId").ToString(),
		objectKey:  utils.Json.Get(body, "result", "objectKey").ToString(),
		token:      utils.Json.Get(body, "result", "token").ToString(),
	}, nil
}

func (u *uploader) publishInfo(resourceId string) error {
	body, err := u.driver.request("https://music.163.com/api/upload/cloud/info/v2", http.MethodPost, ReqOption{
		crypto: "weapi",
		data: map[string]string{
			"md5":        u.md5,
			"filename":   u.filename,
			"song":       u.meta.name,
			"album":      u.meta.album,
			"artist":     u.meta.artist,
			"songid":     u.meta.songId,
			"resourceId": resourceId,
			"bitrate":    "999000",
		},
	})
	if err != nil {
		return err
	}

	_, err = u.driver.request("https://interface.music.163.com/api/cloud/pub/v2", http.MethodPost, ReqOption{
		crypto: "weapi",
		data: map[string]string{
			"songid": utils.Json.Get(body, "songId").ToString(),
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (u *uploader) upload(ctx context.Context, stream model.FileStreamer, up driver.UpdateProgress) error {
	bucket := "jd-musicrep-privatecloud-audio-public"
	token, err := u.allocToken(bucket)
	if err != nil {
		return err
	}

	body, err := u.driver.request("https://wanproxy.127.net/lbs?version=1.0&bucketname="+bucket, http.MethodGet,
		ReqOption{},
	)
	if err != nil {
		return err
	}
	var resp HostsResp
	err = utils.Json.Unmarshal(body, &resp)
	if err != nil {
		return err
	}

	objectKey := strings.ReplaceAll(token.objectKey, "/", "%2F")
	_, err = u.driver.request(
		resp.Upload[0]+"/"+bucket+"/"+objectKey+"?offset=0&complete=true&version=1.0",
		http.MethodPost,
		ReqOption{
			stream: stream,
			up:     up,
			ctx:    ctx,
			headers: map[string]string{
				"x-nos-token":    token.token,
				"Content-Type":   "audio/mpeg",
				"Content-Length": u.size,
				"Content-MD5":    u.md5,
			},
		},
	)
	if err != nil {
		return err
	}

	return nil
}
