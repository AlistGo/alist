package _139

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	streamPkg "github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/cron"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	log "github.com/sirupsen/logrus"
)

type Yun139 struct {
	model.Storage
	Addition
	cron              *cron.Cron
	Account           string
	ref               *Yun139
	PersonalCloudHost string
}

func (d *Yun139) Config() driver.Config {
	return config
}

func (d *Yun139) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Yun139) Init(ctx context.Context) error {
	if d.ref == nil {
		if len(d.Authorization) == 0 {
			return fmt.Errorf("authorization is empty")
		}
		err := d.refreshToken()
		if err != nil {
			return err
		}

		// Query Route Policy
		var resp QueryRoutePolicyResp
		_, err = d.requestRoute(base.Json{
			"userInfo": base.Json{
				"userType":    1,
				"accountType": 1,
				"accountName": d.Account},
			"modAddrType": 1,
		}, &resp)
		if err != nil {
			return err
		}
		for _, policyItem := range resp.Data.RoutePolicyList {
			if policyItem.ModName == "personal" {
				d.PersonalCloudHost = policyItem.HttpsUrl
				break
			}
		}
		if len(d.PersonalCloudHost) == 0 {
			return fmt.Errorf("PersonalCloudHost is empty")
		}

		d.cron = cron.NewCron(time.Hour * 12)
		d.cron.Do(func() {
			err := d.refreshToken()
			if err != nil {
				log.Errorf("%+v", err)
			}
		})
	}
	switch d.Addition.Type {
	case MetaPersonalNew:
		if len(d.Addition.RootFolderID) == 0 {
			d.RootFolderID = "/"
		}
	case MetaPersonal:
		if len(d.Addition.RootFolderID) == 0 {
			d.RootFolderID = "root"
		}
	case MetaGroup:
		if len(d.Addition.RootFolderID) == 0 {
			d.RootFolderID = d.CloudID
		}
	case MetaFamily:
	default:
		return errs.NotImplement
	}
	return nil
}

func (d *Yun139) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*Yun139)
	if ok {
		d.ref = refStorage
		return nil
	}
	return errs.NotSupport
}

func (d *Yun139) Drop(ctx context.Context) error {
	if d.cron != nil {
		d.cron.Stop()
	}
	d.ref = nil
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
	case MetaGroup:
		return d.groupGetFiles(dir.GetID())
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
		url, err = d.getLink(file.GetID())
	case MetaFamily:
		url, err = d.familyGetLink(file.GetID(), file.GetPath())
	case MetaGroup:
		url, err = d.groupGetLink(file.GetID(), file.GetPath())
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
		pathname := "/file/create"
		_, err = d.personalPost(pathname, data, nil)
	case MetaPersonal:
		data := base.Json{
			"createCatalogExtReq": base.Json{
				"parentCatalogID": parentDir.GetID(),
				"newCatalogName":  dirName,
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
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
				"account":     d.getAccount(),
				"accountType": 1,
			},
			"docLibName": dirName,
			"path":       path.Join(parentDir.GetPath(), parentDir.GetID()),
		}
		pathname := "/orchestration/familyCloud-rebuild/cloudCatalog/v1.0/createCloudDoc"
		_, err = d.post(pathname, data, nil)
	case MetaGroup:
		data := base.Json{
			"catalogName": dirName,
			"commonAccountInfo": base.Json{
				"account":     d.getAccount(),
				"accountType": 1,
			},
			"groupID":      d.CloudID,
			"parentFileId": parentDir.GetID(),
			"path":         path.Join(parentDir.GetPath(), parentDir.GetID()),
		}
		pathname := "/orchestration/group-rebuild/catalog/v1.0/createGroupCatalog"
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
		pathname := "/file/batchMove"
		_, err := d.personalPost(pathname, data, nil)
		if err != nil {
			return nil, err
		}
		return srcObj, nil
	case MetaGroup:
		var contentList []string
		var catalogList []string
		if srcObj.IsDir() {
			catalogList = append(catalogList, srcObj.GetID())
		} else {
			contentList = append(contentList, srcObj.GetID())
		}
		data := base.Json{
			"taskType":    3,
			"srcType":     2,
			"srcGroupID":  d.CloudID,
			"destType":    2,
			"destGroupID": d.CloudID,
			"destPath":    dstDir.GetPath(),
			"contentList": contentList,
			"catalogList": catalogList,
			"commonAccountInfo": base.Json{
				"account":     d.getAccount(),
				"accountType": 1,
			},
		}
		pathname := "/orchestration/group-rebuild/task/v1.0/createBatchOprTask"
		_, err := d.post(pathname, data, nil)
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
					"account":     d.getAccount(),
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
		pathname := "/file/update"
		_, err = d.personalPost(pathname, data, nil)
	case MetaPersonal:
		var data base.Json
		var pathname string
		if srcObj.IsDir() {
			data = base.Json{
				"catalogID":   srcObj.GetID(),
				"catalogName": newName,
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
					"accountType": 1,
				},
			}
			pathname = "/orchestration/personalCloud/catalog/v1.0/updateCatalogInfo"
		} else {
			data = base.Json{
				"contentID":   srcObj.GetID(),
				"contentName": newName,
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
					"accountType": 1,
				},
			}
			pathname = "/orchestration/personalCloud/content/v1.0/updateContentInfo"
		}
		_, err = d.post(pathname, data, nil)
	case MetaGroup:
		var data base.Json
		var pathname string
		if srcObj.IsDir() {
			data = base.Json{
				"groupID":           d.CloudID,
				"modifyCatalogID":   srcObj.GetID(),
				"modifyCatalogName": newName,
				"path":              srcObj.GetPath(),
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
					"accountType": 1,
				},
			}
			pathname = "/orchestration/group-rebuild/catalog/v1.0/modifyGroupCatalog"
		} else {
			data = base.Json{
				"groupID":     d.CloudID,
				"contentID":   srcObj.GetID(),
				"contentName": newName,
				"path":        srcObj.GetPath(),
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
					"accountType": 1,
				},
			}
			pathname = "/orchestration/group-rebuild/content/v1.0/modifyGroupContent"
		}
		_, err = d.post(pathname, data, nil)
	case MetaFamily:
		var data base.Json
		var pathname string
		if srcObj.IsDir() {
			// 网页接口不支持重命名家庭云文件夹
			// data = base.Json{
			// 	"catalogType": 3,
			// 	"catalogID":   srcObj.GetID(),
			// 	"catalogName": newName,
			// 	"commonAccountInfo": base.Json{
			// 		"account":     d.getAccount(),
			// 		"accountType": 1,
			// 	},
			// 	"path": srcObj.GetPath(),
			// }
			// pathname = "/orchestration/familyCloud-rebuild/photoContent/v1.0/modifyCatalogInfo"
			return errs.NotImplement
		} else {
			data = base.Json{
				"contentID":   srcObj.GetID(),
				"contentName": newName,
				"commonAccountInfo": base.Json{
					"account":     d.getAccount(),
					"accountType": 1,
				},
				"path": srcObj.GetPath(),
			}
			pathname = "/orchestration/familyCloud-rebuild/photoContent/v1.0/modifyContentInfo"
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
		pathname := "/file/batchCopy"
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
					"account":     d.getAccount(),
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
		pathname := "/recyclebin/batchTrash"
		_, err := d.personalPost(pathname, data, nil)
		return err
	case MetaGroup:
		var contentList []string
		var catalogList []string
		// 必须使用完整路径删除
		if obj.IsDir() {
			catalogList = append(catalogList, obj.GetPath())
		} else {
			contentList = append(contentList, path.Join(obj.GetPath(), obj.GetID()))
		}
		data := base.Json{
			"taskType":    2,
			"srcGroupID":  d.CloudID,
			"contentList": contentList,
			"catalogList": catalogList,
			"commonAccountInfo": base.Json{
				"account":     d.getAccount(),
				"accountType": 1,
			},
		}
		pathname := "/orchestration/group-rebuild/task/v1.0/createBatchOprTask"
		_, err := d.post(pathname, data, nil)
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
					"account":     d.getAccount(),
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
					"account":     d.getAccount(),
					"accountType": 1,
				},
				"sourceCloudID":     d.CloudID,
				"sourceCatalogType": 1002,
				"taskType":          2,
				"path":              obj.GetPath(),
			}
			pathname = "/orchestration/familyCloud-rebuild/batchOprTask/v1.0/createBatchOprTask"
		}
		_, err := d.post(pathname, data, nil)
		return err
	default:
		return errs.NotImplement
	}
}

func (d *Yun139) getPartSize(size int64) int64 {
	if d.CustomUploadPartSize != 0 {
		return d.CustomUploadPartSize
	}
	// 网盘对于分片数量存在上限
	if size/utils.GB > 30 {
		return 512 * utils.MB
	}
	return 100 * utils.MB
}

func (d *Yun139) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	switch d.Addition.Type {
	case MetaPersonalNew:
		var err error
		fullHash := stream.GetHash().GetHash(utils.SHA256)
		if len(fullHash) != utils.SHA256.Width {
			_, fullHash, err = streamPkg.CacheFullInTempFileAndHash(stream, utils.SHA256)
			if err != nil {
				return err
			}
		}

		size := stream.GetSize()
		var partSize = d.getPartSize(size)
		part := size / partSize
		if size%partSize > 0 {
			part++
		} else if part == 0 {
			part = 1
		}
		partInfos := make([]PartInfo, 0, part)
		for i := int64(0); i < part; i++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}
			start := i * partSize
			byteSize := size - start
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

		// 创建任务，获取上传信息和前100个分片的上传地址
		data := base.Json{
			"contentHash":          fullHash,
			"contentHashAlgorithm": "SHA256",
			"contentType":          "application/octet-stream",
			"parallelUpload":       false,
			"partInfos":            firstPartInfos,
			"size":                 size,
			"parentFileId":         dstDir.GetID(),
			"name":                 stream.GetName(),
			"type":                 "file",
			"fileRenameMode":       "auto_rename",
		}
		pathname := "/file/create"
		var resp PersonalUploadResp
		_, err = d.personalPost(pathname, data, &resp)
		if err != nil {
			return err
		}

		// 判断文件是否已存在
		// resp.Data.Exist: true 已存在同名文件且校验相同，云端不会重复增加文件，无需手动处理冲突
		if resp.Data.Exist {
			return nil
		}

		// 判断文件是否支持快传
		// resp.Data.RapidUpload: true 支持快传，但此处直接检测是否返回分片的上传地址
		// 快传的情况下同样需要手动处理冲突
		if resp.Data.PartInfos != nil {
			// 读取前100个分片的上传地址
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
						"account":     d.getAccount(),
						"accountType": 1,
					},
				}
				pathname := "/file/getUploadUrl"
				var moreresp PersonalUploadUrlResp
				_, err = d.personalPost(pathname, moredata, &moreresp)
				if err != nil {
					return err
				}
				uploadPartInfos = append(uploadPartInfos, moreresp.Data.PartInfos...)
			}

			// Progress
			p := driver.NewProgress(size, up)

			rateLimited := driver.NewLimitedUploadStream(ctx, stream)
			// 上传所有分片
			for _, uploadPartInfo := range uploadPartInfos {
				index := uploadPartInfo.PartNumber - 1
				partSize := partInfos[index].PartSize
				log.Debugf("[139] uploading part %+v/%+v", index, len(uploadPartInfos))
				limitReader := io.LimitReader(rateLimited, partSize)

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
			_, err = d.personalPost("/file/complete", data, nil)
			if err != nil {
				return err
			}
		}

		// 处理冲突
		if resp.Data.FileName != stream.GetName() {
			log.Debugf("[139] conflict detected: %s != %s", resp.Data.FileName, stream.GetName())
			// 给服务器一定时间处理数据，避免无法刷新文件列表
			time.Sleep(time.Millisecond * 500)
			// 刷新并获取文件列表
			files, err := d.List(ctx, dstDir, model.ListArgs{Refresh: true})
			if err != nil {
				return err
			}
			// 删除旧文件
			for _, file := range files {
				if file.GetName() == stream.GetName() {
					log.Debugf("[139] conflict: removing old: %s", file.GetName())
					// 删除前重命名旧文件，避免仍旧冲突
					err = d.Rename(ctx, file, stream.GetName()+random.String(4))
					if err != nil {
						return err
					}
					err = d.Remove(ctx, file)
					if err != nil {
						return err
					}
					break
				}
			}
			// 重命名新文件
			for _, file := range files {
				if file.GetName() == resp.Data.FileName {
					log.Debugf("[139] conflict: renaming new: %s => %s", file.GetName(), stream.GetName())
					err = d.Rename(ctx, file, stream.GetName())
					if err != nil {
						return err
					}
					break
				}
			}
		}
		return nil
	case MetaPersonal:
		fallthrough
	case MetaFamily:
		// 处理冲突
		// 获取文件列表
		files, err := d.List(ctx, dstDir, model.ListArgs{})
		if err != nil {
			return err
		}
		// 删除旧文件
		for _, file := range files {
			if file.GetName() == stream.GetName() {
				log.Debugf("[139] conflict: removing old: %s", file.GetName())
				// 删除前重命名旧文件，避免仍旧冲突
				err = d.Rename(ctx, file, stream.GetName()+random.String(4))
				if err != nil {
					return err
				}
				err = d.Remove(ctx, file)
				if err != nil {
					return err
				}
				break
			}
		}
		var reportSize int64
		if d.ReportRealSize {
			reportSize = stream.GetSize()
		} else {
			reportSize = 0
		}
		data := base.Json{
			"manualRename": 2,
			"operation":    0,
			"fileCount":    1,
			"totalSize":    reportSize,
			"uploadContentList": []base.Json{{
				"contentName": stream.GetName(),
				"contentSize": reportSize,
				// "digest": "5a3231986ce7a6b46e408612d385bafa"
			}},
			"parentCatalogID": dstDir.GetID(),
			"newCatalogName":  "",
			"commonAccountInfo": base.Json{
				"account":     d.getAccount(),
				"accountType": 1,
			},
		}
		pathname := "/orchestration/personalCloud/uploadAndDownload/v1.0/pcUploadFileRequest"
		if d.isFamily() {
			data = d.newJson(base.Json{
				"fileCount":    1,
				"manualRename": 2,
				"operation":    0,
				"path":         path.Join(dstDir.GetPath(), dstDir.GetID()),
				"seqNo":        random.String(32), //序列号不能为空
				"totalSize":    reportSize,
				"uploadContentList": []base.Json{{
					"contentName": stream.GetName(),
					"contentSize": reportSize,
					// "digest": "5a3231986ce7a6b46e408612d385bafa"
				}},
			})
			pathname = "/orchestration/familyCloud-rebuild/content/v1.0/getFileUploadURL"
		}
		var resp UploadResp
		_, err = d.post(pathname, data, &resp)
		if err != nil {
			return err
		}
		if resp.Data.Result.ResultCode != "0" {
			return fmt.Errorf("get file upload url failed with result code: %s, message: %s", resp.Data.Result.ResultCode, resp.Data.Result.ResultDesc)
		}

		size := stream.GetSize()
		// Progress
		p := driver.NewProgress(size, up)
		var partSize = d.getPartSize(size)
		part := size / partSize
		if size%partSize > 0 {
			part++
		} else if part == 0 {
			part = 1
		}
		rateLimited := driver.NewLimitedUploadStream(ctx, stream)
		for i := int64(0); i < part; i++ {
			if utils.IsCanceled(ctx) {
				return ctx.Err()
			}

			start := i * partSize
			byteSize := min(size-start, partSize)

			limitReader := io.LimitReader(rateLimited, byteSize)
			// Update Progress
			r := io.TeeReader(limitReader, p)
			req, err := http.NewRequest("POST", resp.Data.UploadResult.RedirectionURL, r)
			if err != nil {
				return err
			}

			req = req.WithContext(ctx)
			req.Header.Set("Content-Type", "text/plain;name="+unicode(stream.GetName()))
			req.Header.Set("contentSize", strconv.FormatInt(size, 10))
			req.Header.Set("range", fmt.Sprintf("bytes=%d-%d", start, start+byteSize-1))
			req.Header.Set("uploadtaskID", resp.Data.UploadResult.UploadTaskID)
			req.Header.Set("rangeType", "0")
			req.ContentLength = byteSize

			res, err := base.HttpClient.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				return fmt.Errorf("unexpected status code: %d", res.StatusCode)
			}
			bodyBytes, err := io.ReadAll(res.Body)
			if err != nil {
				return fmt.Errorf("error reading response body: %v", err)
			}
			var result InterLayerUploadResult
			err = xml.Unmarshal(bodyBytes, &result)
			if err != nil {
				return fmt.Errorf("error parsing XML: %v", err)
			}
			if result.ResultCode != 0 {
				return fmt.Errorf("upload failed with result code: %d, message: %s", result.ResultCode, result.Msg)
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
			uri = "/videoPreview/getPreviewInfo"
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
