package mediatrack

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type MediaTrack struct {
	model.Storage
	Addition
}

func (d *MediaTrack) Config() driver.Config {
	return config
}

func (d *MediaTrack) GetAddition() driver.Additional {
	return d.Addition
}

func (d *MediaTrack) Init(ctx context.Context, storage model.Storage) error {
	d.Storage = storage
	err := utils.Json.UnmarshalFromString(d.Storage.Addition, &d.Addition)
	if err != nil {
		return err
	}
	_, err = d.request("https://kayle.api.mediatrack.cn/users", http.MethodGet, nil, nil)
	return err
}

func (d *MediaTrack) Drop(ctx context.Context) error {
	return nil
}

func (d *MediaTrack) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(f File) (model.Obj, error) {
		size, _ := strconv.ParseInt(f.Size, 10, 64)
		thumb := ""
		if f.File != nil && f.File.Cover != "" {
			thumb = "https://nano.mtres.cn/" + f.File.Cover
		}
		return &model.ObjThumb{
			Object: model.Object{
				ID:       f.ID,
				Name:     f.Title,
				Modified: f.UpdatedAt,
				IsFolder: f.File == nil,
				Size:     size,
			},
			Thumbnail: model.Thumbnail{Thumbnail: thumb},
		}, nil
	})
}

//func (d *MediaTrack) Get(ctx context.Context, path string) (model.Obj, error) {
//	// this is optional
//	return nil, errs.NotImplement
//}

func (d *MediaTrack) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	url := fmt.Sprintf("https://kayn.api.mediatrack.cn/v1/download_token/asset?asset_id=%s&source_type=project&password=&source_id=%s",
		file.GetID(), d.ProjectID)
	log.Debugf("media track url: %s", url)
	body, err := d.request(url, http.MethodGet, nil, nil)
	if err != nil {
		return nil, err
	}
	token := utils.Json.Get(body, "data", "token").ToString()
	url = "https://kayn.api.mediatrack.cn/v1/download/redirect?token=" + token
	res, err := base.NoRedirectClient.R().Get(url)
	if err != nil {
		return nil, err
	}
	log.Debug(res.String())
	link := model.Link{
		URL: url,
	}
	log.Debugln("res code: ", res.StatusCode())
	if res.StatusCode() == 302 {
		link.URL = res.Header().Get("location")
	}
	return &link, nil
}

func (d *MediaTrack) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	url := fmt.Sprintf("https://jayce.api.mediatrack.cn/v3/assets/%s/children", parentDir.GetID())
	_, err := d.request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"type":  1,
			"title": dirName,
		})
	}, nil)
	return err
}

func (d *MediaTrack) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	data := base.Json{
		"parent_id": dstDir.GetID(),
		"ids":       []string{srcObj.GetID()},
	}
	url := "https://jayce.api.mediatrack.cn/v4/assets/batch/move"
	_, err := d.request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	return err
}

func (d *MediaTrack) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	url := "https://jayce.api.mediatrack.cn/v3/assets/" + srcObj.GetID()
	data := base.Json{
		"title": newName,
	}
	_, err := d.request(url, http.MethodPut, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	return err
}

func (d *MediaTrack) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	data := base.Json{
		"parent_id": dstDir.GetID(),
		"ids":       []string{srcObj.GetID()},
	}
	url := "https://jayce.api.mediatrack.cn/v4/assets/batch/clone"
	_, err := d.request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	return err
}

func (d *MediaTrack) Remove(ctx context.Context, obj model.Obj) error {
	dir, err := op.Get(ctx, d, path.Dir(obj.GetPath()))
	if err != nil {
		return err
	}
	data := base.Json{
		"origin_id": dir.GetID(),
		"ids":       []string{obj.GetID()},
	}
	url := "https://jayce.api.mediatrack.cn/v4/assets/batch/delete"
	_, err = d.request(url, http.MethodDelete, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	return err
}

func (d *MediaTrack) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	src := "assets/" + uuid.New().String()
	var resp UploadResp
	_, err := d.request("https://jayce.api.mediatrack.cn/v3/storage/tokens/asset", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("src", src)
	}, &resp)
	if err != nil {
		return err
	}
	credential := resp.Data.Credentials
	cfg := &aws.Config{
		Credentials: credentials.NewStaticCredentials(credential.TmpSecretID, credential.TmpSecretKey, credential.Token),
		Region:      &resp.Data.Region,
		Endpoint:    aws.String("cos.accelerate.myqcloud.com"),
	}
	s, err := session.NewSession(cfg)
	if err != nil {
		return err
	}
	tempFile, err := utils.CreateTempFile(stream.GetReadCloser())
	if err != nil {
		return err
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()
	uploader := s3manager.NewUploader(s)
	input := &s3manager.UploadInput{
		Bucket: &resp.Data.Bucket,
		Key:    &resp.Data.Object,
		Body:   tempFile,
	}
	_, err = uploader.Upload(input)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://jayce.api.mediatrack.cn/v3/assets/%s/children", dstDir.GetID())
	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	h := md5.New()
	_, err = io.Copy(h, tempFile)
	if err != nil {
		return err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	data := base.Json{
		"category":    0,
		"description": stream.GetName(),
		"hash":        hash,
		"mime":        stream.GetMimetype(),
		"size":        stream.GetSize(),
		"src":         src,
		"title":       stream.GetName(),
		"type":        0,
	}
	_, err = d.request(url, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, nil)
	return err
}

var _ driver.Driver = (*MediaTrack)(nil)
