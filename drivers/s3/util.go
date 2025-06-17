package s3

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	log "github.com/sirupsen/logrus"
)

// do others that not defined in Driver interface

func (d *S3) initSession() error {
	var err error
	accessKeyID, secretAccessKey, sessionToken := d.AccessKeyID, d.SecretAccessKey, d.SessionToken
	if d.config.Name == "Doge" {
		credentialsTmp, err := getCredentials(d.AccessKeyID, d.SecretAccessKey)
		if err != nil {
			return err
		}
		accessKeyID, secretAccessKey, sessionToken = credentialsTmp.AccessKeyId, credentialsTmp.SecretAccessKey, credentialsTmp.SessionToken
	}
	cfg := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(accessKeyID, secretAccessKey, sessionToken),
		Region:           &d.Region,
		Endpoint:         &d.Endpoint,
		S3ForcePathStyle: aws.Bool(d.ForcePathStyle),
	}
	d.Session, err = session.NewSession(cfg)
	return err
}

func (d *S3) getClient(link bool) *s3.S3 {
	client := s3.New(d.Session)
	if link && d.CustomHost != "" {
		client.Handlers.Build.PushBack(func(r *request.Request) {
			if r.HTTPRequest.Method != http.MethodGet {
				return
			}
			//判断CustomHost是否以http://或https://开头
			split := strings.SplitN(d.CustomHost, "://", 2)
			if utils.SliceContains([]string{"http", "https"}, split[0]) {
				r.HTTPRequest.URL.Scheme = split[0]
				r.HTTPRequest.URL.Host = split[1]
			} else {
				r.HTTPRequest.URL.Host = d.CustomHost
			}
		})
	}
	return client
}

func getKey(path string, dir bool) string {
	path = strings.TrimPrefix(path, "/")
	if path != "" && dir {
		path += "/"
	}
	return path
}

var defaultPlaceholderName = ".alist"

func getPlaceholderName(placeholder string) string {
	if placeholder == "" {
		return defaultPlaceholderName
	}
	return placeholder
}

func (d *S3) listV1(prefix string, args model.ListArgs) ([]model.Obj, error) {
	prefix = getKey(prefix, true)
	log.Debugf("list: %s", prefix)
	files := make([]model.Obj, 0)
	marker := ""
	for {
		input := &s3.ListObjectsInput{
			Bucket:    &d.Bucket,
			Marker:    &marker,
			Prefix:    &prefix,
			Delimiter: aws.String("/"),
		}
		listObjectsResult, err := d.client.ListObjects(input)
		if err != nil {
			return nil, err
		}
		for _, object := range listObjectsResult.CommonPrefixes {
			name := path.Base(strings.Trim(*object.Prefix, "/"))
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Modified: d.Modified,
				IsFolder: true,
			}
			files = append(files, &file)
		}
		for _, object := range listObjectsResult.Contents {
			name := path.Base(*object.Key)
			if !args.S3ShowPlaceholder && (name == getPlaceholderName(d.Placeholder) || name == d.Placeholder) {
				continue
			}
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Size:     *object.Size,
				Modified: *object.LastModified,
			}
			files = append(files, &file)
		}
		if listObjectsResult.IsTruncated == nil {
			return nil, errors.New("IsTruncated nil")
		}
		if *listObjectsResult.IsTruncated {
			marker = *listObjectsResult.NextMarker
		} else {
			break
		}
	}
	return files, nil
}

func (d *S3) listV2(prefix string, args model.ListArgs) ([]model.Obj, error) {
	prefix = getKey(prefix, true)
	files := make([]model.Obj, 0)
	var continuationToken, startAfter *string
	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            &d.Bucket,
			ContinuationToken: continuationToken,
			Prefix:            &prefix,
			Delimiter:         aws.String("/"),
			StartAfter:        startAfter,
		}
		listObjectsResult, err := d.client.ListObjectsV2(input)
		if err != nil {
			return nil, err
		}
		log.Debugf("resp: %+v", listObjectsResult)
		for _, object := range listObjectsResult.CommonPrefixes {
			name := path.Base(strings.Trim(*object.Prefix, "/"))
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Modified: d.Modified,
				IsFolder: true,
			}
			files = append(files, &file)
		}
		for _, object := range listObjectsResult.Contents {
			if strings.HasSuffix(*object.Key, "/") {
				continue
			}
			name := path.Base(*object.Key)
			if !args.S3ShowPlaceholder && (name == getPlaceholderName(d.Placeholder) || name == d.Placeholder) {
				continue
			}
			file := model.Object{
				//Id:        *object.Key,
				Name:     name,
				Size:     *object.Size,
				Modified: *object.LastModified,
			}
			files = append(files, &file)
		}
		if !aws.BoolValue(listObjectsResult.IsTruncated) {
			break
		}
		if listObjectsResult.NextContinuationToken != nil {
			continuationToken = listObjectsResult.NextContinuationToken
			continue
		}
		if len(listObjectsResult.Contents) == 0 {
			break
		}
		startAfter = listObjectsResult.Contents[len(listObjectsResult.Contents)-1].Key
	}
	return files, nil
}

func (d *S3) copy(ctx context.Context, src string, dst string, isDir bool) error {
	if isDir {
		return d.copyDir(ctx, src, dst)
	}
	return d.copyFile(ctx, src, dst)
}

func (d *S3) copyFile(ctx context.Context, src string, dst string) error {
	srcKey := getKey(src, false)
	dstKey := getKey(dst, false)
	input := &s3.CopyObjectInput{
		Bucket:     &d.Bucket,
		CopySource: aws.String(url.PathEscape(d.Bucket + "/" + srcKey)),
		Key:        &dstKey,
	}
	_, err := d.client.CopyObject(input)
	return err
}

func (d *S3) copyDir(ctx context.Context, src string, dst string) error {
	objs, err := op.List(ctx, d, src, model.ListArgs{S3ShowPlaceholder: true})
	if err != nil {
		return err
	}
	for _, obj := range objs {
		cSrc := path.Join(src, obj.GetName())
		cDst := path.Join(dst, obj.GetName())
		if obj.IsDir() {
			err = d.copyDir(ctx, cSrc, cDst)
		} else {
			err = d.copyFile(ctx, cSrc, cDst)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *S3) removeDir(ctx context.Context, src string) error {
	objs, err := op.List(ctx, d, src, model.ListArgs{})
	if err != nil {
		return err
	}
	for _, obj := range objs {
		cSrc := path.Join(src, obj.GetName())
		if obj.IsDir() {
			err = d.removeDir(ctx, cSrc)
		} else {
			err = d.removeFile(cSrc)
		}
		if err != nil {
			return err
		}
	}
	_ = d.removeFile(path.Join(src, getPlaceholderName(d.Placeholder)))
	_ = d.removeFile(path.Join(src, d.Placeholder))
	return nil
}

func (d *S3) removeFile(src string) error {
	key := getKey(src, false)
	input := &s3.DeleteObjectInput{
		Bucket: &d.Bucket,
		Key:    &key,
	}
	_, err := d.client.DeleteObject(input)
	return err
}
