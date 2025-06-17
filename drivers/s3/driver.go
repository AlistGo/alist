package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	stdpath "path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/cron"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	log "github.com/sirupsen/logrus"
)

type S3 struct {
	model.Storage
	Addition
	Session    *session.Session
	client     *s3.S3
	linkClient *s3.S3

	config driver.Config
	cron   *cron.Cron
}

func (d *S3) Config() driver.Config {
	return d.config
}

func (d *S3) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *S3) Init(ctx context.Context) error {
	if d.Region == "" {
		d.Region = "alist"
	}
	if d.config.Name == "Doge" {
		// 多吉云每次临时生成的秘钥有效期为 2h，所以这里设置为 118 分钟重新生成一次
		d.cron = cron.NewCron(time.Minute * 118)
		d.cron.Do(func() {
			err := d.initSession()
			if err != nil {
				log.Errorln("Doge init session error:", err)
			}
			d.client = d.getClient(false)
			d.linkClient = d.getClient(true)
		})
	}
	err := d.initSession()
	if err != nil {
		return err
	}
	d.client = d.getClient(false)
	d.linkClient = d.getClient(true)
	return nil
}

func (d *S3) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	return nil
}

func (d *S3) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if d.ListObjectVersion == "v2" {
		return d.listV2(dir.GetPath(), args)
	}
	return d.listV1(dir.GetPath(), args)
}

func (d *S3) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	path := getKey(file.GetPath(), false)
	filename := stdpath.Base(path)
	disposition := fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename))
	if d.AddFilenameToDisposition {
		disposition = fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename))
	}
	input := &s3.GetObjectInput{
		Bucket: &d.Bucket,
		Key:    &path,
		//ResponseContentDisposition: &disposition,
	}
	if d.CustomHost == "" {
		input.ResponseContentDisposition = &disposition
	}
	req, _ := d.linkClient.GetObjectRequest(input)
	var link model.Link
	var err error
	if d.CustomHost != "" {
		if d.EnableCustomHostPresign {
			link.URL, err = req.Presign(time.Hour * time.Duration(d.SignURLExpire))
		} else {
			err = req.Build()
			link.URL = req.HTTPRequest.URL.String()
		}
		if d.RemoveBucket {
			link.URL = strings.Replace(link.URL, "/"+d.Bucket, "", 1)
		}
	} else {
		if common.ShouldProxy(d, filename) {
			err = req.Sign()
			link.URL = req.HTTPRequest.URL.String()
			link.Header = req.HTTPRequest.Header
		} else {
			link.URL, err = req.Presign(time.Hour * time.Duration(d.SignURLExpire))
		}
	}
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func (d *S3) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.Put(ctx, &model.Object{
		Path: stdpath.Join(parentDir.GetPath(), dirName),
	}, &stream.FileStream{
		Obj: &model.Object{
			Name:     getPlaceholderName(d.Placeholder),
			Modified: time.Now(),
		},
		Reader:   io.NopCloser(bytes.NewReader([]byte{})),
		Mimetype: "application/octet-stream",
	}, func(float64) {})
}

func (d *S3) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	err := d.Copy(ctx, srcObj, dstDir)
	if err != nil {
		return err
	}
	return d.Remove(ctx, srcObj)
}

func (d *S3) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	err := d.copy(ctx, srcObj.GetPath(), stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName), srcObj.IsDir())
	if err != nil {
		return err
	}
	return d.Remove(ctx, srcObj)
}

func (d *S3) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return d.copy(ctx, srcObj.GetPath(), stdpath.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.IsDir())
}

func (d *S3) Remove(ctx context.Context, obj model.Obj) error {
	if obj.IsDir() {
		return d.removeDir(ctx, obj.GetPath())
	}
	return d.removeFile(obj.GetPath())
}

func (d *S3) Put(ctx context.Context, dstDir model.Obj, s model.FileStreamer, up driver.UpdateProgress) error {
	uploader := s3manager.NewUploader(d.Session)
	if s.GetSize() > s3manager.MaxUploadParts*s3manager.DefaultUploadPartSize {
		uploader.PartSize = s.GetSize() / (s3manager.MaxUploadParts - 1)
	}
	key := getKey(stdpath.Join(dstDir.GetPath(), s.GetName()), false)
	contentType := s.GetMimetype()
	log.Debugln("key:", key)
	input := &s3manager.UploadInput{
		Bucket: &d.Bucket,
		Key:    &key,
		Body: driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
			Reader:         s,
			UpdateProgress: up,
		}),
		ContentType: &contentType,
	}
	_, err := uploader.UploadWithContext(ctx, input)
	return err
}

var _ driver.Driver = (*S3)(nil)
