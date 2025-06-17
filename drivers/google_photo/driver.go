package google_photo

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type GooglePhoto struct {
	model.Storage
	Addition
	AccessToken string
}

func (d *GooglePhoto) Config() driver.Config {
	return config
}

func (d *GooglePhoto) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *GooglePhoto) Init(ctx context.Context) error {
	return d.refreshToken()
}

func (d *GooglePhoto) Drop(ctx context.Context) error {
	return nil
}

func (d *GooglePhoto) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src MediaItem) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *GooglePhoto) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	f, err := d.getMedia(file.GetID())
	if err != nil {
		return nil, err
	}

	if strings.Contains(f.MimeType, "image/") {
		return &model.Link{
			URL: f.BaseURL + "=d",
		}, nil
	} else if strings.Contains(f.MimeType, "video/") {
		return &model.Link{
			URL: f.BaseURL + "=dv",
		}, nil
	}
	return &model.Link{}, nil
}

func (d *GooglePhoto) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return errs.NotSupport
}

func (d *GooglePhoto) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *GooglePhoto) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return errs.NotSupport
}

func (d *GooglePhoto) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotSupport
}

func (d *GooglePhoto) Remove(ctx context.Context, obj model.Obj) error {
	return errs.NotSupport
}

func (d *GooglePhoto) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	var e Error
	// Create resumable upload url
	postHeaders := map[string]string{
		"Authorization":              "Bearer " + d.AccessToken,
		"Content-type":               "application/octet-stream",
		"X-Goog-Upload-Command":      "start",
		"X-Goog-Upload-Content-Type": stream.GetMimetype(),
		"X-Goog-Upload-Protocol":     "resumable",
		"X-Goog-Upload-Raw-Size":     strconv.FormatInt(stream.GetSize(), 10),
	}
	url := "https://photoslibrary.googleapis.com/v1/uploads"
	res, err := base.NoRedirectClient.R().SetHeaders(postHeaders).
		SetError(&e).
		Post(url)

	if err != nil {
		return err
	}
	if e.Error.Code != 0 {
		if e.Error.Code == 401 {
			err = d.refreshToken()
			if err != nil {
				return err
			}
			return d.Put(ctx, dstDir, stream, up)
		}
		return fmt.Errorf("%s: %v", e.Error.Message, e.Error.Errors)
	}

	//Upload to the Google Photo
	postUrl := res.Header().Get("X-Goog-Upload-URL")
	//chunkSize := res.Header().Get("X-Goog-Upload-Chunk-Granularity")
	postHeaders = map[string]string{
		"X-Goog-Upload-Command": "upload, finalize",
		"X-Goog-Upload-Offset":  "0",
	}

	resp, err := d.request(postUrl, http.MethodPost, func(req *resty.Request) {
		req.SetBody(driver.NewLimitedUploadStream(ctx, stream)).SetContext(ctx)
	}, nil, postHeaders)

	if err != nil {
		return err
	}
	//Create MediaItem
	createItemUrl := "https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate"

	postHeaders = map[string]string{
		"X-Goog-Upload-Command": "upload, finalize",
		"X-Goog-Upload-Offset":  "0",
	}

	data := base.Json{
		"newMediaItems": []base.Json{
			{
				"description": "item-description",
				"simpleMediaItem": base.Json{
					"fileName":    stream.GetName(),
					"uploadToken": string(resp),
				},
			},
		},
	}

	_, err = d.request(createItemUrl, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, nil, postHeaders)

	return err
}

var _ driver.Driver = (*GooglePhoto)(nil)
