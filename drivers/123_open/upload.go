package _123_open

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/errgroup"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/go-resty/resty/v2"
)

func (d *Open123) create(parentFileID int64, filename string, etag string, size int64, duplicate int, containDir bool) (*UploadCreateResp, error) {
	var resp UploadCreateResp
	_, err := d.Request(UploadCreate, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"parentFileId": parentFileID,
			"filename":     filename,
			"etag":         strings.ToLower(etag),
			"size":         size,
			"duplicate":    duplicate,
			"containDir":   containDir,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Open123) url(preuploadID string, sliceNo int64) (string, error) {
	// get upload url
	var resp UploadUrlResp
	_, err := d.Request(UploadUrl, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"preuploadId": preuploadID,
			"sliceNo":     sliceNo,
		})
	}, &resp)
	if err != nil {
		return "", err
	}
	return resp.Data.PresignedURL, nil
}

func (d *Open123) complete(preuploadID string) (*UploadCompleteResp, error) {
	var resp UploadCompleteResp
	_, err := d.Request(UploadComplete, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"preuploadID": preuploadID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Open123) async(preuploadID string) (*UploadAsyncResp, error) {
	var resp UploadAsyncResp
	_, err := d.Request(UploadAsync, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"preuploadID": preuploadID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Open123) Upload(ctx context.Context, file model.FileStreamer, createResp *UploadCreateResp, up driver.UpdateProgress) error {
	size := file.GetSize()
	chunkSize := createResp.Data.SliceSize
	uploadNums := (size + chunkSize - 1) / chunkSize
	threadG, uploadCtx := errgroup.NewGroupWithContext(ctx, d.UploadThread,
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay))

	for partIndex := int64(0); partIndex < uploadNums; partIndex++ {
		if utils.IsCanceled(uploadCtx) {
			return ctx.Err()
		}
		partIndex := partIndex
		partNumber := partIndex + 1 // 分片号从1开始
		offset := partIndex * chunkSize
		size := min(chunkSize, size-offset)
		limitedReader, err := file.RangeRead(http_range.Range{
			Start:  offset,
			Length: size})
		if err != nil {
			return err
		}
		limitedReader = driver.NewLimitedUploadStream(ctx, limitedReader)

		threadG.Go(func(ctx context.Context) error {
			uploadPartUrl, err := d.url(createResp.Data.PreuploadID, partNumber)
			if err != nil {
				return err
			}

			req, err := http.NewRequestWithContext(ctx, "PUT", uploadPartUrl, limitedReader)
			if err != nil {
				return err
			}
			req = req.WithContext(ctx)
			req.ContentLength = size

			res, err := base.HttpClient.Do(req)
			if err != nil {
				return err
			}
			_ = res.Body.Close()

			progress := 10.0 + 85.0*float64(threadG.Success())/float64(uploadNums)
			up(progress)
			return nil
		})
	}

	if err := threadG.Wait(); err != nil {
		return err
	}

	uploadCompleteResp, err := d.complete(createResp.Data.PreuploadID)
	if err != nil {
		return err
	}
	if uploadCompleteResp.Data.Async == false || uploadCompleteResp.Data.Completed {
		return nil
	}

	for {
		uploadAsyncResp, err := d.async(createResp.Data.PreuploadID)
		if err != nil {
			return err
		}
		if uploadAsyncResp.Data.Completed {
			break
		}
	}
	up(100)
	return nil
}
