package aliyundrive_open

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	streamPkg "github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

func makePartInfos(size int) []base.Json {
	partInfoList := make([]base.Json, size)
	for i := 0; i < size; i++ {
		partInfoList[i] = base.Json{"part_number": 1 + i}
	}
	return partInfoList
}

func calPartSize(fileSize int64) int64 {
	var partSize int64 = 20 * utils.MB
	if fileSize > partSize {
		if fileSize > 1*utils.TB { // file Size over 1TB
			partSize = 5 * utils.GB // file part size 5GB
		} else if fileSize > 768*utils.GB { // over 768GB
			partSize = 109951163 // ≈ 104.8576MB, split 1TB into 10,000 part
		} else if fileSize > 512*utils.GB { // over 512GB
			partSize = 82463373 // ≈ 78.6432MB
		} else if fileSize > 384*utils.GB { // over 384GB
			partSize = 54975582 // ≈ 52.4288MB
		} else if fileSize > 256*utils.GB { // over 256GB
			partSize = 41231687 // ≈ 39.3216MB
		} else if fileSize > 128*utils.GB { // over 128GB
			partSize = 27487791 // ≈ 26.2144MB
		}
	}
	return partSize
}

func (d *AliyundriveOpen) getUploadUrl(count int, fileId, uploadId string) ([]PartInfo, error) {
	partInfoList := makePartInfos(count)
	var resp CreateResp
	_, err := d.request("/adrive/v1.0/openFile/getUploadUrl", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":       d.DriveId,
			"file_id":        fileId,
			"part_info_list": partInfoList,
			"upload_id":      uploadId,
		}).SetResult(&resp)
	})
	return resp.PartInfoList, err
}

func (d *AliyundriveOpen) uploadPart(ctx context.Context, r io.Reader, partInfo PartInfo) error {
	uploadUrl := partInfo.UploadUrl
	if d.InternalUpload {
		uploadUrl = strings.ReplaceAll(uploadUrl, "https://cn-beijing-data.aliyundrive.net/", "http://ccp-bj29-bj-1592982087.oss-cn-beijing-internal.aliyuncs.com/")
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadUrl, r)
	if err != nil {
		return err
	}
	res, err := base.HttpClient.Do(req)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusConflict {
		return fmt.Errorf("upload status: %d", res.StatusCode)
	}
	return nil
}

func (d *AliyundriveOpen) completeUpload(fileId, uploadId string) (model.Obj, error) {
	// 3. complete
	var newFile File
	_, err := d.request("/adrive/v1.0/openFile/complete", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":  d.DriveId,
			"file_id":   fileId,
			"upload_id": uploadId,
		}).SetResult(&newFile)
	})
	if err != nil {
		return nil, err
	}
	return fileToObj(newFile), nil
}

type ProofRange struct {
	Start int64
	End   int64
}

func getProofRange(input string, size int64) (*ProofRange, error) {
	if size == 0 {
		return &ProofRange{}, nil
	}
	tmpStr := utils.GetMD5EncodeStr(input)[0:16]
	tmpInt, err := strconv.ParseUint(tmpStr, 16, 64)
	if err != nil {
		return nil, err
	}
	index := tmpInt % uint64(size)
	pr := &ProofRange{
		Start: int64(index),
		End:   int64(index) + 8,
	}
	if pr.End >= size {
		pr.End = size
	}
	return pr, nil
}

func (d *AliyundriveOpen) calProofCode(stream model.FileStreamer) (string, error) {
	proofRange, err := getProofRange(d.getAccessToken(), stream.GetSize())
	if err != nil {
		return "", err
	}
	length := proofRange.End - proofRange.Start
	reader, err := stream.RangeRead(http_range.Range{Start: proofRange.Start, Length: length})
	if err != nil {
		return "", err
	}
	buf := make([]byte, length)
	n, err := io.ReadFull(reader, buf)
	if err == io.ErrUnexpectedEOF {
		return "", fmt.Errorf("can't read data, expected=%d, got=%d", len(buf), n)
	}
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func (d *AliyundriveOpen) upload(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// 1. create
	// Part Size Unit: Bytes, Default: 20MB,
	// Maximum number of slices 10,000, ≈195.3125GB
	var partSize = calPartSize(stream.GetSize())
	const dateFormat = "2006-01-02T15:04:05.000Z"
	mtimeStr := stream.ModTime().UTC().Format(dateFormat)
	ctimeStr := stream.CreateTime().UTC().Format(dateFormat)

	createData := base.Json{
		"drive_id":          d.DriveId,
		"parent_file_id":    dstDir.GetID(),
		"name":              stream.GetName(),
		"type":              "file",
		"check_name_mode":   "ignore",
		"local_modified_at": mtimeStr,
		"local_created_at":  ctimeStr,
	}
	count := int(math.Ceil(float64(stream.GetSize()) / float64(partSize)))
	createData["part_info_list"] = makePartInfos(count)
	// rapid upload
	rapidUpload := !stream.IsForceStreamUpload() && stream.GetSize() > 100*utils.KB && d.RapidUpload
	if rapidUpload {
		log.Debugf("[aliyundrive_open] start cal pre_hash")
		// read 1024 bytes to calculate pre hash
		reader, err := stream.RangeRead(http_range.Range{Start: 0, Length: 1024})
		if err != nil {
			return nil, err
		}
		hash, err := utils.HashReader(utils.SHA1, reader)
		if err != nil {
			return nil, err
		}
		createData["size"] = stream.GetSize()
		createData["pre_hash"] = hash
	}
	var createResp CreateResp
	_, err, e := d.requestReturnErrResp("/adrive/v1.0/openFile/create", http.MethodPost, func(req *resty.Request) {
		req.SetBody(createData).SetResult(&createResp)
	})
	if err != nil {
		if e.Code != "PreHashMatched" || !rapidUpload {
			return nil, err
		}
		log.Debugf("[aliyundrive_open] pre_hash matched, start rapid upload")

		hash := stream.GetHash().GetHash(utils.SHA1)
		if len(hash) != utils.SHA1.Width {
			_, hash, err = streamPkg.CacheFullInTempFileAndHash(stream, utils.SHA1)
			if err != nil {
				return nil, err
			}
		}

		delete(createData, "pre_hash")
		createData["proof_version"] = "v1"
		createData["content_hash_name"] = "sha1"
		createData["content_hash"] = hash
		createData["proof_code"], err = d.calProofCode(stream)
		if err != nil {
			return nil, fmt.Errorf("cal proof code error: %s", err.Error())
		}
		_, err = d.request("/adrive/v1.0/openFile/create", http.MethodPost, func(req *resty.Request) {
			req.SetBody(createData).SetResult(&createResp)
		})
		if err != nil {
			return nil, err
		}
	}

	if !createResp.RapidUpload {
		// 2. normal upload
		log.Debugf("[aliyundive_open] normal upload")

		preTime := time.Now()
		var offset, length int64 = 0, partSize
		//var length
		for i := 0; i < len(createResp.PartInfoList); i++ {
			if utils.IsCanceled(ctx) {
				return nil, ctx.Err()
			}
			// refresh upload url if 50 minutes passed
			if time.Since(preTime) > 50*time.Minute {
				createResp.PartInfoList, err = d.getUploadUrl(count, createResp.FileId, createResp.UploadId)
				if err != nil {
					return nil, err
				}
				preTime = time.Now()
			}
			if remain := stream.GetSize() - offset; length > remain {
				length = remain
			}
			rd := utils.NewMultiReadable(io.LimitReader(stream, partSize))
			if rapidUpload {
				srd, err := stream.RangeRead(http_range.Range{Start: offset, Length: length})
				if err != nil {
					return nil, err
				}
				rd = utils.NewMultiReadable(srd)
			}
			err = retry.Do(func() error {
				_ = rd.Reset()
				rateLimitedRd := driver.NewLimitedUploadStream(ctx, rd)
				return d.uploadPart(ctx, rateLimitedRd, createResp.PartInfoList[i])
			},
				retry.Attempts(3),
				retry.DelayType(retry.BackOffDelay),
				retry.Delay(time.Second))
			if err != nil {
				return nil, err
			}
			offset += partSize
			up(float64(i*100) / float64(count))
		}
	} else {
		log.Debugf("[aliyundrive_open] rapid upload success, file id: %s", createResp.FileId)
	}

	log.Debugf("[aliyundrive_open] create file success, resp: %+v", createResp)
	// 3. complete
	return d.completeUpload(createResp.FileId, createResp.UploadId)
}
