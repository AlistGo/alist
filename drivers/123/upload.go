package _123

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

func (d *Pan123) getS3PreSignedUrls(ctx context.Context, upReq *UploadResp, start, end int) (*S3PreSignedURLs, error) {
	data := base.Json{
		"bucket":          upReq.Data.Bucket,
		"key":             upReq.Data.Key,
		"partNumberEnd":   end,
		"partNumberStart": start,
		"uploadId":        upReq.Data.UploadId,
		"StorageNode":     upReq.Data.StorageNode,
	}
	var s3PreSignedUrls S3PreSignedURLs
	_, err := d.Request(S3PreSignedUrls, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data).SetContext(ctx)
	}, &s3PreSignedUrls)
	if err != nil {
		return nil, err
	}
	return &s3PreSignedUrls, nil
}

func (d *Pan123) getS3Auth(ctx context.Context, upReq *UploadResp, start, end int) (*S3PreSignedURLs, error) {
	data := base.Json{
		"StorageNode":     upReq.Data.StorageNode,
		"bucket":          upReq.Data.Bucket,
		"key":             upReq.Data.Key,
		"partNumberEnd":   end,
		"partNumberStart": start,
		"uploadId":        upReq.Data.UploadId,
	}
	var s3PreSignedUrls S3PreSignedURLs
	_, err := d.Request(S3Auth, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data).SetContext(ctx)
	}, &s3PreSignedUrls)
	if err != nil {
		return nil, err
	}
	return &s3PreSignedUrls, nil
}

func (d *Pan123) completeS3(ctx context.Context, upReq *UploadResp, file model.FileStreamer, isMultipart bool) error {
	data := base.Json{
		"StorageNode": upReq.Data.StorageNode,
		"bucket":      upReq.Data.Bucket,
		"fileId":      upReq.Data.FileId,
		"fileSize":    file.GetSize(),
		"isMultipart": isMultipart,
		"key":         upReq.Data.Key,
		"uploadId":    upReq.Data.UploadId,
	}
	_, err := d.Request(UploadCompleteV2, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data).SetContext(ctx)
	}, nil)
	return err
}

func (d *Pan123) newUpload(ctx context.Context, upReq *UploadResp, file model.FileStreamer, up driver.UpdateProgress) error {
	tmpF, err := file.CacheFullInTempFile()
	if err != nil {
		return err
	}
	// fetch s3 pre signed urls
	size := file.GetSize()
	chunkSize := min(size, 16*utils.MB)
	chunkCount := int(size / chunkSize)
	lastChunkSize := size % chunkSize
	if lastChunkSize > 0 {
		chunkCount++
	} else {
		lastChunkSize = chunkSize
	}
	// only 1 batch is allowed
	batchSize := 1
	getS3UploadUrl := d.getS3Auth
	if chunkCount > 1 {
		batchSize = 10
		getS3UploadUrl = d.getS3PreSignedUrls
	}
	for i := 1; i <= chunkCount; i += batchSize {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		start := i
		end := min(i+batchSize, chunkCount+1)
		s3PreSignedUrls, err := getS3UploadUrl(ctx, upReq, start, end)
		if err != nil {
			return err
		}
		// upload each chunk
		for j := start; j < end; j++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}
			curSize := chunkSize
			if j == chunkCount {
				curSize = lastChunkSize
			}
			err = d.uploadS3Chunk(ctx, upReq, s3PreSignedUrls, j, end, io.NewSectionReader(tmpF, chunkSize*int64(j-1), curSize), curSize, false, getS3UploadUrl)
			if err != nil {
				return err
			}
			up(float64(j) * 100 / float64(chunkCount))
		}
	}
	// complete s3 upload
	return d.completeS3(ctx, upReq, file, chunkCount > 1)
}

func (d *Pan123) uploadS3Chunk(ctx context.Context, upReq *UploadResp, s3PreSignedUrls *S3PreSignedURLs, cur, end int, reader *io.SectionReader, curSize int64, retry bool, getS3UploadUrl func(ctx context.Context, upReq *UploadResp, start int, end int) (*S3PreSignedURLs, error)) error {
	uploadUrl := s3PreSignedUrls.Data.PreSignedUrls[strconv.Itoa(cur)]
	if uploadUrl == "" {
		return fmt.Errorf("upload url is empty, s3PreSignedUrls: %+v", s3PreSignedUrls)
	}
	req, err := http.NewRequest("PUT", uploadUrl, driver.NewLimitedUploadStream(ctx, reader))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.ContentLength = curSize
	//req.Header.Set("Content-Length", strconv.FormatInt(curSize, 10))
	res, err := base.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusForbidden {
		if retry {
			return fmt.Errorf("upload s3 chunk %d failed, status code: %d", cur, res.StatusCode)
		}
		// refresh s3 pre signed urls
		newS3PreSignedUrls, err := getS3UploadUrl(ctx, upReq, cur, end)
		if err != nil {
			return err
		}
		s3PreSignedUrls.Data.PreSignedUrls = newS3PreSignedUrls.Data.PreSignedUrls
		// retry
		reader.Seek(0, io.SeekStart)
		return d.uploadS3Chunk(ctx, upReq, s3PreSignedUrls, cur, end, reader, curSize, true, getS3UploadUrl)
	}
	if res.StatusCode != http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("upload s3 chunk %d failed, status code: %d, body: %s", cur, res.StatusCode, body)
	}
	return nil
}
