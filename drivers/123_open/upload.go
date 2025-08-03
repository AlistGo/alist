package _123Open

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"golang.org/x/sync/errgroup"
	"io"
	"mime/multipart"
	"net/http"
	"runtime"
	"strconv"
	"time"
)

func (d *Open123) create(parentFileID int64, filename, etag string, size int64, duplicate int, containDir bool) (*UploadCreateResp, error) {
	var resp UploadCreateResp

	_, err := d.Request(ApiCreateUploadURL, http.MethodPost, func(req *resty.Request) {
		body := base.Json{
			"parentFileID": parentFileID,
			"filename":     filename,
			"etag":         etag,
			"size":         size,
		}
		if duplicate > 0 {
			body["duplicate"] = duplicate
		}
		if containDir {
			body["containDir"] = true
		}
		req.SetBody(body)
	}, &resp)

	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Open123) GetUploadDomains() ([]string, error) {
	var resp struct {
		Code    int      `json:"code"`
		Message string   `json:"message"`
		Data    []string `json:"data"`
	}

	_, err := d.Request(ApiUploadDomainURL, http.MethodGet, nil, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("get upload domain failed: %s", resp.Message)
	}
	return resp.Data, nil
}

func (d *Open123) UploadSingle(ctx context.Context, createResp *UploadCreateResp, file model.FileStreamer, parentID int64) error {
	domain := createResp.Data.Servers[0]

	etag := file.GetHash().GetHash(utils.MD5)
	if len(etag) < utils.MD5.Width {
		_, _, err := stream.CacheFullInTempFileAndHash(file, utils.MD5)
		if err != nil {
			return err
		}
	}

	reader, err := file.RangeRead(http_range.Range{Start: 0, Length: file.GetSize()})
	if err != nil {
		return err
	}
	reader = driver.NewLimitedUploadStream(ctx, reader)

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	mw.WriteField("parentFileID", fmt.Sprint(parentID))
	mw.WriteField("filename", file.GetName())
	mw.WriteField("etag", etag)
	mw.WriteField("size", fmt.Sprint(file.GetSize()))
	fw, _ := mw.CreateFormFile("file", file.GetName())
	_, err = io.Copy(fw, reader)
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", domain+ApiSingleUploadURL, &b)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.tm.accessToken)
	req.Header.Set("Platform", "open_platform")
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			FileID    int64 `json:"fileID"`
			Completed bool  `json:"completed"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unmarshal response error: %v, body: %s", err, string(body))
	}
	if result.Code != 0 {
		return fmt.Errorf("upload failed: %s", result.Message)
	}
	if !result.Data.Completed || result.Data.FileID == 0 {
		return fmt.Errorf("upload incomplete or missing fileID")
	}
	return nil
}

func (d *Open123) Upload(ctx context.Context, file model.FileStreamer, parentID int64, createResp *UploadCreateResp, up driver.UpdateProgress) error {
	if cacher, ok := file.(interface{ CacheFullInTempFile() (model.File, error) }); ok {
		if _, err := cacher.CacheFullInTempFile(); err != nil {
			return err
		}
	}

	size := file.GetSize()
	chunkSize := createResp.Data.SliceSize
	uploadNums := (size + chunkSize - 1) / chunkSize
	uploadDomain := createResp.Data.Servers[0]

	if d.UploadThread <= 0 {
		cpuCores := runtime.NumCPU()
		threads := cpuCores * 2
		if threads < 4 {
			threads = 4
		}
		if threads > 16 {
			threads = 16
		}
		d.UploadThread = threads
		fmt.Printf("[Upload] Auto set upload concurrency: %d (CPU cores=%d)\n", d.UploadThread, cpuCores)
	}

	fmt.Printf("[Upload] File size: %d bytes, chunk size: %d bytes, total slices: %d, concurrency: %d\n",
		size, chunkSize, uploadNums, d.UploadThread)

	if size <= 1<<30 {
		return d.UploadSingle(ctx, createResp, file, parentID)
	}

	if createResp.Data.Reuse {
		up(100)
		return nil
	}

	client := resty.New()
	semaphore := make(chan struct{}, d.UploadThread)
	threadG, _ := errgroup.WithContext(ctx)

	var progressArr = make([]int64, uploadNums)

	for partIndex := int64(0); partIndex < uploadNums; partIndex++ {
		partIndex := partIndex
		semaphore <- struct{}{}

		threadG.Go(func() error {
			defer func() { <-semaphore }()
			offset := partIndex * chunkSize
			length := min(chunkSize, size-offset)
			partNumber := partIndex + 1

			fmt.Printf("[Slice %d] Starting read from offset %d, length %d\n", partNumber, offset, length)
			reader, err := file.RangeRead(http_range.Range{Start: offset, Length: length})
			if err != nil {
				return fmt.Errorf("[Slice %d] RangeRead error: %v", partNumber, err)
			}

			buf := make([]byte, length)
			n, err := io.ReadFull(reader, buf)
			if err != nil && err != io.EOF {
				return fmt.Errorf("[Slice %d] Read error: %v", partNumber, err)
			}
			buf = buf[:n]
			hash := md5.Sum(buf)
			sliceMD5Str := hex.EncodeToString(hash[:])

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			writer.WriteField("preuploadID", createResp.Data.PreuploadID)
			writer.WriteField("sliceNo", strconv.FormatInt(partNumber, 10))
			writer.WriteField("sliceMD5", sliceMD5Str)
			partName := fmt.Sprintf("%s.part%d", file.GetName(), partNumber)
			fw, _ := writer.CreateFormFile("slice", partName)
			fw.Write(buf)
			writer.Close()

			resp, err := client.R().
				SetHeader("Authorization", "Bearer "+d.tm.accessToken).
				SetHeader("Platform", "open_platform").
				SetHeader("Content-Type", writer.FormDataContentType()).
				SetBody(body.Bytes()).
				Post(uploadDomain + ApiUploadSliceURL)

			if err != nil {
				return fmt.Errorf("[Slice %d] Upload HTTP error: %v", partNumber, err)
			}
			if resp.StatusCode() != 200 {
				return fmt.Errorf("[Slice %d] Upload failed with status: %s, resp: %s", partNumber, resp.Status(), resp.String())
			}

			progressArr[partIndex] = length
			var totalUploaded int64 = 0
			for _, v := range progressArr {
				totalUploaded += v
			}
			if up != nil {
				percent := float64(totalUploaded) / float64(size) * 100
				up(percent)
			}

			fmt.Printf("[Slice %d] MD5: %s\n", partNumber, sliceMD5Str)
			fmt.Printf("[Slice %d] Upload finished\n", partNumber)
			return nil
		})
	}

	if err := threadG.Wait(); err != nil {
		return err
	}

	var completeResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Completed bool  `json:"completed"`
			FileID    int64 `json:"fileID"`
		} `json:"data"`
	}

	for {
		reqBody := fmt.Sprintf(`{"preuploadID":"%s"}`, createResp.Data.PreuploadID)
		req, err := http.NewRequestWithContext(ctx, "POST", uploadDomain+ApiUploadCompleteURL, bytes.NewBufferString(reqBody))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+d.tm.accessToken)
		req.Header.Set("Platform", "open_platform")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err := json.Unmarshal(body, &completeResp); err != nil {
			return fmt.Errorf("completion response unmarshal error: %v, body: %s", err, string(body))
		}
		if completeResp.Code != 0 {
			return fmt.Errorf("completion API returned error code %d: %s", completeResp.Code, completeResp.Message)
		}
		if completeResp.Data.Completed && completeResp.Data.FileID != 0 {
			fmt.Printf("[Upload] Upload completed successfully. FileID: %d\n", completeResp.Data.FileID)
			break
		}
		time.Sleep(time.Second)
	}
	up(100)
	return nil
}
