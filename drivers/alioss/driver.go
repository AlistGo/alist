package alioss

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	log "github.com/sirupsen/logrus"
)

type AliOSS struct {
	model.Storage
	Addition
	client *oss.Client
	bucket *oss.Bucket
}

func (d *AliOSS) Config() driver.Config {
	return config
}

func (d *AliOSS) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *AliOSS) Init(ctx context.Context) error {
	// 创建OSS客户端
	client, err := oss.New(d.Endpoint, d.AccessKeyId, d.AccessKeySecret)
	if err != nil {
		return fmt.Errorf("failed to create OSS client: %w", err)
	}
	d.client = client

	// 获取Bucket
	bucket, err := client.Bucket(d.BucketName)
	if err != nil {
		return fmt.Errorf("failed to get bucket: %w", err)
	}
	d.bucket = bucket

	return nil
}

func (d *AliOSS) Drop(ctx context.Context) error {
	return nil
}

func (d *AliOSS) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	prefix := d.getKey(dir.GetPath(), true)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var objects []model.Obj
	marker := ""
	for {
		lsRes, err := d.bucket.ListObjects(oss.Prefix(prefix), oss.Delimiter("/"), oss.Marker(marker))
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		// 处理文件
		for _, object := range lsRes.Objects {
			if object.Key == prefix {
				continue // 跳过目录本身
			}
			obj := &model.Object{
				Name:     stdpath.Base(object.Key),
				Size:     object.Size,
				Modified: object.LastModified,
				IsFolder: false,
			}
			objects = append(objects, obj)
		}

		// 处理目录
		for _, prefix := range lsRes.CommonPrefixes {
			obj := &model.Object{
				Name:     stdpath.Base(strings.TrimSuffix(prefix, "/")),
				Size:     0,
				Modified: time.Now(),
				IsFolder: true,
			}
			objects = append(objects, obj)
		}

		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
	}

	return objects, nil
}

func (d *AliOSS) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	key := d.getKey(file.GetPath(), false)
	filename := stdpath.Base(key)

	// 生成签名URL
	signedURL, err := d.bucket.SignURL(key, oss.HTTPGet, int64(d.SignURLExpire)*3600)
	if err != nil {
		return nil, fmt.Errorf("failed to sign URL: %w", err)
	}

	// 设置Content-Disposition头
	disposition := fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename))

	header := make(http.Header)
	header.Set("Content-Disposition", disposition)

	link := &model.Link{
		URL:    signedURL,
		Header: header,
	}

	// 如果需要代理下载
	if common.ShouldProxy(d, filename) {
		// 这里可以添加代理逻辑
		log.Debugf("Proxy download for file: %s", filename)
	}

	return link, nil
}

func (d *AliOSS) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	// OSS不支持真正的目录，我们创建一个占位符文件
	placeholderName := d.getPlaceholderName()
	key := d.getKey(stdpath.Join(parentDir.GetPath(), dirName, placeholderName), false)

	// 创建空的占位符文件
	err := d.bucket.PutObject(key, bytes.NewReader([]byte{}))
	if err != nil {
		return fmt.Errorf("failed to create placeholder: %w", err)
	}

	return nil
}

func (d *AliOSS) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	// 先复制，再删除
	err := d.Copy(ctx, srcObj, dstDir)
	if err != nil {
		return err
	}
	return d.Remove(ctx, srcObj)
}

func (d *AliOSS) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	srcKey := d.getKey(srcObj.GetPath(), srcObj.IsDir())
	dstKey := d.getKey(stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName), srcObj.IsDir())

	// 复制到新位置
	_, err := d.bucket.CopyObject(srcKey, dstKey)
	if err != nil {
		return fmt.Errorf("failed to copy object: %w", err)
	}

	// 删除原文件
	return d.Remove(ctx, srcObj)
}

func (d *AliOSS) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcKey := d.getKey(srcObj.GetPath(), srcObj.IsDir())
	dstKey := d.getKey(stdpath.Join(dstDir.GetPath(), srcObj.GetName()), srcObj.IsDir())

	_, err := d.bucket.CopyObject(srcKey, dstKey)
	if err != nil {
		return fmt.Errorf("failed to copy object: %w", err)
	}

	return nil
}

func (d *AliOSS) Remove(ctx context.Context, obj model.Obj) error {
	key := d.getKey(obj.GetPath(), obj.IsDir())

	if obj.IsDir() {
		// 删除目录下的所有文件
		prefix := key
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}

		marker := ""
		for {
			lsRes, err := d.bucket.ListObjects(oss.Prefix(prefix), oss.Marker(marker))
			if err != nil {
				return fmt.Errorf("failed to list objects for deletion: %w", err)
			}

			var keys []string
			for _, object := range lsRes.Objects {
				keys = append(keys, object.Key)
			}

			if len(keys) > 0 {
				_, err = d.bucket.DeleteObjects(keys)
				if err != nil {
					return fmt.Errorf("failed to delete objects: %w", err)
				}
			}

			if !lsRes.IsTruncated {
				break
			}
			marker = lsRes.NextMarker
		}
	} else {
		// 删除单个文件
		err := d.bucket.DeleteObject(key)
		if err != nil {
			return fmt.Errorf("failed to delete object: %w", err)
		}
	}

	return nil
}

func (d *AliOSS) Put(ctx context.Context, dstDir model.Obj, streamer model.FileStreamer, up driver.UpdateProgress) error {
	key := d.getKey(stdpath.Join(dstDir.GetPath(), streamer.GetName()), false)

	// 创建上传选项
	options := []oss.Option{
		oss.ContentType(streamer.GetMimetype()),
	}

	// 上传文件
	err := d.bucket.PutObject(key, &driver.ReaderUpdatingProgress{
		Reader:         streamer,
		UpdateProgress: up,
	}, options...)

	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}

	return nil
}

// 辅助方法
func (d *AliOSS) getKey(path string, isDir bool) string {
	key := strings.TrimPrefix(path, "/")
	if isDir && key != "" && !strings.HasSuffix(key, "/") {
		key += "/"
	}
	return key
}

func (d *AliOSS) getPlaceholderName() string {
	if d.Placeholder != "" {
		return d.Placeholder
	}
	return ".placeholder"
}

var _ driver.Driver = (*AliOSS)(nil)
