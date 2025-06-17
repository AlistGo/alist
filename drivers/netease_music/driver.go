package netease_music

import (
	"context"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	_ "golang.org/x/image/webp"
)

type NeteaseMusic struct {
	model.Storage
	Addition

	csrfToken     string
	musicU        string
	fileMapByName map[string]model.Obj
}

func (d *NeteaseMusic) Config() driver.Config {
	return config
}

func (d *NeteaseMusic) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *NeteaseMusic) Init(ctx context.Context) error {
	d.csrfToken = d.Addition.getCookie("__csrf")
	d.musicU = d.Addition.getCookie("MUSIC_U")

	if d.csrfToken == "" || d.musicU == "" {
		return errs.EmptyToken
	}

	return nil
}

func (d *NeteaseMusic) Drop(ctx context.Context) error {
	return nil
}

func (d *NeteaseMusic) Get(ctx context.Context, path string) (model.Obj, error) {
	if path == "/" {
		return &model.Object{
			IsFolder: true,
			Path:     path,
		}, nil
	}

	fragments := strings.Split(path, "/")
	if len(fragments) > 1 {
		fileName := fragments[1]
		if strings.HasSuffix(fileName, ".lrc") {
			lrc := d.fileMapByName[fileName]
			return d.getLyricObj(lrc)
		}
		if song, ok := d.fileMapByName[fileName]; ok {
			return song, nil
		} else {
			return nil, errs.ObjectNotFound
		}
	}

	return nil, errs.ObjectNotFound
}

func (d *NeteaseMusic) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return d.getSongObjs(args)
}

func (d *NeteaseMusic) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if lrc, ok := file.(*LyricObj); ok {
		if args.Type == "parsed" {
			return lrc.getLyricLink(), nil
		} else {
			return lrc.getProxyLink(args), nil
		}
	}

	return d.getSongLink(file)
}

func (d *NeteaseMusic) Remove(ctx context.Context, obj model.Obj) error {
	return d.removeSongObj(obj)
}

func (d *NeteaseMusic) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	return d.putSongStream(ctx, stream, up)
}

func (d *NeteaseMusic) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *NeteaseMusic) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *NeteaseMusic) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return errs.NotSupport
}

func (d *NeteaseMusic) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return errs.NotSupport
}

var _ driver.Driver = (*NeteaseMusic)(nil)
