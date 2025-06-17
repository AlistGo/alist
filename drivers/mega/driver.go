package mega

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/pquerna/otp/totp"
	"github.com/rclone/rclone/lib/readers"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/t3rm1n4l/go-mega"
)

type Mega struct {
	model.Storage
	Addition
	c *mega.Mega
}

func (d *Mega) Config() driver.Config {
	return config
}

func (d *Mega) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Mega) Init(ctx context.Context) error {
	var twoFACode = d.TwoFACode
	d.c = mega.New()
	if d.TwoFASecret != "" {
		code, err := totp.GenerateCode(d.TwoFASecret, time.Now())
		if err != nil {
			return fmt.Errorf("generate totp code failed: %w", err)
		}
		twoFACode = code
	}
	return d.c.MultiFactorLogin(d.Email, d.Password, twoFACode)
}

func (d *Mega) Drop(ctx context.Context) error {
	return nil
}

func (d *Mega) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if node, ok := dir.(*MegaNode); ok {
		nodes, err := d.c.FS.GetChildren(node.n)
		if err != nil {
			return nil, err
		}
		fn := make(map[string]model.Obj)
		for i := range nodes {
			n := nodes[i]
			if n.GetType() != mega.FILE && n.GetType() != mega.FOLDER {
				continue
			}
			if _, ok := fn[n.GetName()]; !ok {
				fn[n.GetName()] = &MegaNode{n}
			} else if sameNameObj := fn[n.GetName()]; (&MegaNode{n}).ModTime().After(sameNameObj.ModTime()) {
				fn[n.GetName()] = &MegaNode{n}
			}
		}
		res := make([]model.Obj, 0)
		for _, v := range fn {
			res = append(res, v)
		}
		return res, nil
	}
	log.Errorf("can't convert: %+v", dir)
	return nil, fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) GetRoot(ctx context.Context) (model.Obj, error) {
	n := d.c.FS.GetRoot()
	log.Debugf("mega root: %+v", *n)
	return &MegaNode{n}, nil
}

func (d *Mega) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if node, ok := file.(*MegaNode); ok {

		//down, err := d.c.NewDownload(n.Node)
		//if err != nil {
		//	return nil, fmt.Errorf("open download file failed: %w", err)
		//}

		size := file.GetSize()
		resultRangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
			length := httpRange.Length
			if httpRange.Length >= 0 && httpRange.Start+httpRange.Length >= size {
				length = -1
			}
			var down *mega.Download
			err := utils.Retry(3, time.Second, func() (err error) {
				down, err = d.c.NewDownload(node.n)
				return err
			})
			if err != nil {
				return nil, fmt.Errorf("open download file failed: %w", err)
			}
			oo := &openObject{
				ctx:  ctx,
				d:    down,
				skip: httpRange.Start,
			}

			return readers.NewLimitedReadCloser(oo, length), nil
		}
		resultRangeReadCloser := &model.RangeReadCloser{RangeReader: resultRangeReader}
		resultLink := &model.Link{
			RangeReadCloser: resultRangeReadCloser,
		}
		return resultLink, nil
	}
	return nil, fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if parentNode, ok := parentDir.(*MegaNode); ok {
		_, err := d.c.CreateDir(dirName, parentNode.n)
		return err
	}
	return fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcNode, ok := srcObj.(*MegaNode); ok {
		if dstNode, ok := dstDir.(*MegaNode); ok {
			return d.c.Move(srcNode.n, dstNode.n)
		}
	}
	return fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if srcNode, ok := srcObj.(*MegaNode); ok {
		return d.c.Rename(srcNode.n, newName)
	}
	return fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	return errs.NotImplement
}

func (d *Mega) Remove(ctx context.Context, obj model.Obj) error {
	if node, ok := obj.(*MegaNode); ok {
		return d.c.Delete(node.n, false)
	}
	return fmt.Errorf("unable to convert dir to mega n")
}

func (d *Mega) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	if dstNode, ok := dstDir.(*MegaNode); ok {
		u, err := d.c.NewUpload(dstNode.n, stream.GetName(), stream.GetSize())
		if err != nil {
			return err
		}

		reader := driver.NewLimitedUploadStream(ctx, stream)
		for id := 0; id < u.Chunks(); id++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}
			_, chkSize, err := u.ChunkLocation(id)
			if err != nil {
				return err
			}
			chunk := make([]byte, chkSize)
			n, err := io.ReadFull(reader, chunk)
			if err != nil && err != io.EOF {
				return err
			}
			if n != len(chunk) {
				return errors.New("chunk too short")
			}

			err = u.UploadChunk(id, chunk)
			if err != nil {
				return err
			}
			up(float64(id) * 100 / float64(u.Chunks()))
		}

		_, err = u.Finish()
		return err
	}
	return fmt.Errorf("unable to convert dir to mega n")
}

//func (d *Mega) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Mega)(nil)
