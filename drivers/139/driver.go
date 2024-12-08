package _139

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/cron"
	log "github.com/sirupsen/logrus"
)

type Yun139 struct {
	model.Storage
	Addition
	cron   *cron.Cron
	Account string
}

func (d *Yun139) Config() driver.Config {
	return config
}

func (d *Yun139) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Yun139) Init(ctx context.Context) error {
	if d.Authorization == "" {
		return fmt.Errorf("authorization is empty")
	}
	d.cron = cron.NewCron(time.Hour * 24 * 7)
	d.cron.Do(func() {
		err := d.refreshToken()
		if err != nil {
			log.Errorf("%+v", err)
		}
	})
	switch d.Addition.Type {
	case MetaPersonalNew:
		if len(d.Addition.RootFolderID) == 0 {
			d.RootFolderID = "/"
		}
		return nil
	case MetaPersonal:
		if len(d.Addition.RootFolderID) == 0 {
			d.RootFolderID = "root"
		}
		fallthrough
	case MetaFamily:
		decode, err := base64.StdEncoding.DecodeString(d.Authorization)
		if err != nil {
			return err
		}
		decodeStr := string(decode)
		splits := strings.Split(decodeStr, ":")
		if len(splits) < 2 {
			return fmt.Errorf("authorization is invalid, splits < 2")
		}
		d.Account = splits[1]
		_, err = d.post("/orchestration/personalCloud/user/v1.0/qryUserExternInfo", base.Json{
			"qryUserExternInfoReq": base.Json{
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			},
		}, nil)
		return err
	default:
		return errs.NotImplement
	}
}

func (d *Yun139) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	return nil
}

func (d *Yun139) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	switch d.Addition.Type {
	case MetaPersonalNew:
		return d.personalGetFiles(dir.GetID())
	case MetaPersonal:
		return d.getFiles(dir.GetID())
	case MetaFamily:
		return d.familyGetFiles(dir.GetID())
	default:
		return nil, errs.NotImplement
	}
}

func (d *Yun139) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var url string
	var err error
	switch d.Addition.Type {
	case MetaPersonalNew:
		url, err = d.personalGetLink(file.GetID())
	case MetaPersonal:
		fallthrough
	case MetaFamily:
		url, err = d.getLink(file.GetID())
	default:
		return nil, errs.NotImplement
	}
	if err != nil {
		return nil, err
	}
	return &model.Link{URL: url}, nil
}

func (d *Yun139) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	var err error
	switch d.Addition.Type {
	case MetaPersonalNew:
		data := base.Json{
			"parentFileId":   parentDir.GetID(),
			"name":           dirName,
			"description":    "",
			"type":           "folder",
			"fileRenameMode": "force_rename",
		}
		pathname := "/hcy/file/create"
		_, err = d.personalPost(pathname, data, nil)
	case MetaPersonal:
		data := base.Json{
			"createCatalogExtReq": base.Json{
				"parentCatalogID": parentDir.GetID(),
				"newCatalogName":  dirName,
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			},
		}
		pathname := "/orchestration/personalCloud/catalog/v1.0/createCatalogExt"
		_, err = d.post(pathname, data, nil)
	case MetaFamily:
		data := base.Json{
			"cloudID": d.CloudID,
			"commonAccountInfo": base.Json{
				"account":     d.Account,
				"accountType": 1,
			},
			"docLibName": dirName,
		}
		pathname := "/orchestration/familyCloud/cloudCatalog/v1.0/createCloudDoc"
		_, err = d.post(pathname, data, nil)
	default:
		err = errs.NotImplement
	}
	return err
}

func (d *Yun139) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	switch d.Addition.Type {
	case MetaPersonalNew:
		data := base.Json{
			"fileIds":        []string{srcObj.GetID()},
			"toParentFileId": dstDir.GetID(),
		}
		pathname := "/hcy/file/batchMove"
		_, err := d.personalPost(pathname, data, nil)
		if err != nil {
			return nil, err
		}
		return srcObj, nil
	case MetaPersonal:
		var contentInfoList []string
		var catalogInfoList []string
		if srcObj.IsDir() {
			catalogInfoList = append(catalogInfoList, srcObj.GetID())
		} else {
			contentInfoList = append(contentInfoList, srcObj.GetID())
		}
		data := base.Json{
			"createBatchOprTaskReq": base.Json{
				"taskType":   3,
				"actionType": "304",
				"taskInfo": base.Json{
					"contentInfoList": contentInfoList,
					"catalogInfoList": catalogInfoList,
					"newCatalogID":    dstDir.GetID(),
				},
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			},
		}
		pathname := "/orchestration/personalCloud/batchOprTask/v1.0/createBatchOprTask"
		_, err := d.post(pathname, data, nil)
		if err != nil {
			return nil, err
		}
		return srcObj, nil
	default:
		return nil, errs.NotImplement
	}
}

func (d *Yun139) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	var err error
	switch d.Addition.Type {
	case MetaPersonalNew:
		data := base.Json{
			"fileId":      srcObj.GetID(),
			"name":        newName,
			"description": "",
		}
		pathname := "/hcy/file/update"
		_, err = d.personalPost(pathname, data, nil)
	case MetaPersonal:
		var data base.Json
		var pathname string
		if srcObj.IsDir() {
			data = base.Json{
				"catalogID":   srcObj.GetID(),
				"catalogName": newName,
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			}
			pathname = "/orchestration/personalCloud/catalog/v1.0/updateCatalogInfo"
		} else {
			data = base.Json{
				"contentID":   srcObj.GetID(),
				"contentName": newName,
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			}
			pathname = "/orchestration/personalCloud/content/v1.0/updateContentInfo"
		}
		_, err = d.post(pathname, data, nil)
	default:
		err = errs.NotImplement
	}
	return err
}

func (d *Yun139) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	var err error
	switch d.Addition.Type {
	case MetaPersonalNew:
		data := base.Json{
			"fileIds":        []string{srcObj.GetID()},
			"toParentFileId": dstDir.GetID(),
		}
		pathname := "/hcy/file/batchCopy"
		_, err := d.personalPost(pathname, data, nil)
		return err
	case MetaPersonal:
		var contentInfoList []string
		var catalogInfoList []string
		if srcObj.IsDir() {
			catalogInfoList = append(catalogInfoList, srcObj.GetID())
		} else {
			contentInfoList = append(contentInfoList, srcObj.GetID())
		}
		data := base.Json{
			"createBatchOprTaskReq": base.Json{
				"taskType":   3,
				"actionType": 309,
				"taskInfo": base.Json{
					"contentInfoList": contentInfoList,
					"catalogInfoList": catalogInfoList,
					"newCatalogID":    dstDir.GetID(),
				},
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			},
		}
		pathname := "/orchestration/personalCloud/batchOprTask/v1.0/createBatchOprTask"
		_, err = d.post(pathname, data, nil)
	default:
		err = errs.NotImplement
	}
	return err
}

func (d *Yun139) Remove(ctx context.Context, obj model.Obj) error {
	switch d.Addition.Type {
	case MetaPersonalNew:
		data := base.Json{
			"fileIds": []string{obj.GetID()},
		}
		pathname := "/hcy/recyclebin/batchTrash"
		_, err := d.personalPost(pathname, data, nil)
		return err
	case MetaPersonal:
		fallthrough
	case MetaFamily:
		var contentInfoList []string
		var catalogInfoList []string
		if obj.IsDir() {
			catalogInfoList = append(catalogInfoList, obj.GetID())
		} else {
			contentInfoList = append(contentInfoList, obj.GetID())
		}
		data := base.Json{
			"createBatchOprTaskReq": base.Json{
				"taskType":   2,
				"actionType": 201,
				"taskInfo": base.Json{
					"newCatalogID":    "",
					"contentInfoList": contentInfoList,
					"catalogInfoList": catalogInfoList,
				},
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			},
		}
		pathname := "/orchestration/personalCloud/batchOprTask/v1.0/createBatchOprTask"
		if d.isFamily() {
			data = base.Json{
				"catalogList": catalogInfoList,
				"contentList": contentInfoList,
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
				"sourceCatalogType": 1002,
				"taskType":          2,
			}
			pathname = "/orchestration/familyCloud/batchOprTask/v1.0/createBatchOprTask"
		}
		_, err := d.post(pathname, data, nil)
		return err
	default:
		return errs.NotImplement
	}
}

const (
	_  = iota //ignore first value by assigning to blank identifier
	KB = 1 << (10 * iota)
	MB
	GB
	TB
)

func (d *Yun139) getPartSize(size int64) int64 {
	if d.CustomUploadPartSize != 0 {
		return d.CustomUploadPartSize
	}
	// 网盘对于分片数量存在上限
	if size/GB > 30 {
		return 512 * MB
	}
	return 100 * MB
}

func (d *Yun139) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	switch d.Addition.Type {
	case MetaPersonalNew:
		var err error
		fullHash := stream.GetHash().GetHash(utils.SHA256)
		if len(fullHash) <= 0 {
			tmpF, err := stream.CacheFullInTempFile()
			if err != nil {
				return err
			}
			fullHash, err = utils.HashFile(utils.SHA256, tmpF)
			if err != nil {
				return err
			}
		}

		partInfos := []PartInfo{}
		var partSize = d.getPartSize(stream.GetSize())
		part := (stream.GetSize() + partSize - 1) / partSize
		if part == 0 {
			part = 1
		}
		for i := int64(0); i < part; i++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}
			start := i * partSize
			byteSize := stream.GetSize() - start
			if byteSize > partSize {
				byteSize = partSize
			}
			partNumber := i + 1
			partInfo := PartInfo{
				PartNumber: partNumber,
				PartSize:   byteSize,
				ParallelHashCtx: ParallelHashCtx{
					PartOffset: start,
				},
			}
			partInfos = append(partInfos, partInfo)
		}

		// 筛选出前 100 个 partInfos
		firstPartInfos := partInfos
		if len(firstPartInfos) > 100 {
			firstPartInfos = firstPartInfos[:100]
		}

		// 获取上传信息和前100个分片的上传地址
		data := base.Json{
			"contentHash":          fullHash,
			"contentHashAlgorithm": "SHA256",
			"contentType":          "application/octet-stream",
			"parallelUpload":       false,
			"partInfos":            firstPartInfos,
			"size":                 stream.GetSize(),
			"parentFileId":         dstDir.GetID(),
			"name":                 stream.GetName(),
			"type":                 "file",
			"fileRenameMode":       "auto_rename",
		}
		pathname := "/hcy/file/create"
		var resp PersonalUploadResp
		_, err = d.personalPost(pathname, data, &resp)
		if err != nil {
			return err
		}

		if resp.Data.Exist || resp.Data.RapidUpload {
			return nil
		}

		uploadPartInfos := resp.Data.PartInfos

		// 获取后续分片的上传地址
		for i := 101; i < len(partInfos); i += 100 {
			end := i + 100
			if end > len(partInfos) {
				end = len(partInfos)
			}
			batchPartInfos := partInfos[i:end]

			moredata := base.Json{
				"fileId":    resp.Data.FileId,
				"uploadId":  resp.Data.UploadId,
				"partInfos": batchPartInfos,
				"commonAccountInfo": base.Json{
					"account":     d.Account,
					"accountType": 1,
				},
			}
			pathname := "/hcy/file/getUploadUrl"
			var moreresp PersonalUploadUrlResp
			_, err = d.personalPost(pathname, moredata, &moreresp)
			if err != nil {
				return err
			}
			uploadPartInfos = append(uploadPartInfos, moreresp.Data.PartInfos...)
		}

		// Progress
		p := driver.NewProgress(stream.GetSize(), up)

		// 上传所有分片
		for _, uploadPartInfo := range uploadPartInfos {
			index := uploadPartInfo.PartNumber - 1
			partSize := partInfos[index].PartSize
			log.Debugf("[139] uploading part %+v/%+v", index, len(uploadPartInfos))
			limitReader := io.LimitReader(stream, partSize)

			// Update Progress
			r := io.TeeReader(limitReader, p)

			req, err := http.NewRequest("PUT", uploadPartInfo.UploadUrl, r)
			if err != nil {
				return err
			}
			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Header.Set("Content-Length", fmt.Sprint(partSize))
			req.Header.Set("Origin", "https://yun.139.com")
			req.Header.Set("Referer", "https://yun.139.com/")
			req.ContentLength = partSize

			res, err := base.HttpClient.Do(req)
			if err != nil {
				return err
			}
			_ = res.Body.Close()
			log.Debugf("[139] uploaded: %+v", res)
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", res.StatusCode)
			}
		}

		data = base.Json{
			"contentHash":          fullHash,
			"contentHashAlgorithm": "SHA256",
			"fileId":               resp.Data.FileId,
			"uploadId":             resp.Data.UploadId,
		}
		_, err = d.personalPost("/hcy/file/complete", data, nil)
		if err != nil {
			return err
		}
		return nil
	case MetaPersonal:
		fallthrough
	case MetaFamily:
		data := base.Json{
			"manualRename": 2,
			"operation":    0,
			"fileCount":    1,
			"totalSize":    0, // 去除上传大小限制
			"uploadContentList": []base.Json{{
				"contentName": stream.GetName(),
				"contentSize": 0, // 去除上传大小限制
				// "digest": "5a3231986ce7a6b46e408612d385bafa"
			}},
			"parentCatalogID": dstDir.GetID(),
			"newCatalogName":  "",
			"commonAccountInfo": base.Json{
				"account":     d.Account,
				"accountType": 1,
			},
		}
		pathname := "/orchestration/personalCloud/uploadAndDownload/v1.0/pcUploadFileRequest"
		if d.isFamily() {
			// data = d.newJson(base.Json{
			// 	"fileCount":    1,
			// 	"manualRename": 2,
			// 	"operation":    0,
			// 	"path":         "",
			// 	"seqNo":        "",
			// 	"totalSize":    0,
			// 	"uploadContentList": []base.Json{{
			// 		"contentName": stream.GetName(),
			// 		"contentSize": 0,
			// 		// "digest": "5a3231986ce7a6b46e408612d385bafa"
			// 	}},
			// })
			// pathname = "/orchestration/familyCloud/content/v1.0/getFileUploadURL"
			return errs.NotImplement
		}
		var resp UploadResp
		_, err := d.post(pathname, data, &resp)
		if err != nil {
			return err
		}

		// Progress
		p := driver.NewProgress(stream.GetSize(), up)

		var partSize = d.getPartSize(stream.GetSize())
		part := (stream.GetSize() + partSize - 1) / partSize
		if part == 0 {
			part = 1
		}
		for i := int64(0); i < part; i++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}

			start := i * partSize
			byteSize := stream.GetSize() - start
			if byteSize > partSize {
				byteSize = partSize
			}

			limitReader := io.LimitReader(stream, byteSize)
			// Update Progress
			r := io.TeeReader(limitReader, p)
			req, err := http.NewRequest("POST", resp.Data.UploadResult.RedirectionURL, r)
			if err != nil {
				return err
			}

			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "text/plain;name="+unicode(stream.GetName()))
			req.Header.Set("contentSize", strconv.FormatInt(stream.GetSize(), 10))
			req.Header.Set("range", fmt.Sprintf("bytes=%d-%d", start, start+byteSize-1))
			req.Header.Set("uploadtaskID", resp.Data.UploadResult.UploadTaskID)
			req.Header.Set("rangeType", "0")
			req.ContentLength = byteSize

			res, err := base.HttpClient.Do(req)
			if err != nil {
				return err
			}
			_ = res.Body.Close()
			log.Debugf("%+v", res)
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", res.StatusCode)
			}
		}

		return nil
	default:
		return errs.NotImplement
	}
}

func (d *Yun139) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	switch d.Addition.Type {
	case MetaPersonalNew:
		var resp base.Json
		var uri string
		data := base.Json{
			"category": "video",
			"fileId":   args.Obj.GetID(),
		}
		switch args.Method {
		case "video_preview":
			uri = "/hcy/videoPreview/getPreviewInfo"
		default:
			return nil, errs.NotSupport
		}
		_, err := d.personalPost(uri, data, &resp)
		if err != nil {
			return nil, err
		}
		return resp["data"], nil
	default:
		return nil, errs.NotImplement
	}
}

var _ driver.Driver = (*Yun139)(nil)
