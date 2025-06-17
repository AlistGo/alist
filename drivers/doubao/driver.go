package doubao

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
)

type Doubao struct {
	model.Storage
	Addition
	*UploadToken
	UserId       string
	uploadThread int
}

func (d *Doubao) Config() driver.Config {
	return config
}

func (d *Doubao) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Doubao) Init(ctx context.Context) error {
	// TODO login / refresh token
	//op.MustSaveDriverStorage(d)
	uploadThread, err := strconv.Atoi(d.UploadThread)
	if err != nil || uploadThread < 1 {
		d.uploadThread, d.UploadThread = 3, "3" // Set default value
	} else {
		d.uploadThread = uploadThread
	}

	if d.UserId == "" {
		userInfo, err := d.getUserInfo()
		if err != nil {
			return err
		}

		d.UserId = strconv.FormatInt(userInfo.UserID, 10)
	}

	if d.UploadToken == nil {
		uploadToken, err := d.initUploadToken()
		if err != nil {
			return err
		}

		d.UploadToken = uploadToken
	}

	return nil
}

func (d *Doubao) Drop(ctx context.Context) error {
	return nil
}

func (d *Doubao) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var files []model.Obj
	fileList, err := d.getFiles(dir.GetID(), "")
	if err != nil {
		return nil, err
	}

	for _, child := range fileList {
		files = append(files, &Object{
			Object: model.Object{
				ID:       child.ID,
				Path:     child.ParentID,
				Name:     child.Name,
				Size:     child.Size,
				Modified: time.Unix(child.UpdateTime, 0),
				Ctime:    time.Unix(child.CreateTime, 0),
				IsFolder: child.NodeType == 1,
			},
			Key:      child.Key,
			NodeType: child.NodeType,
		})
	}

	return files, nil
}

func (d *Doubao) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var downloadUrl string

	if u, ok := file.(*Object); ok {
		switch d.DownloadApi {
		case "get_download_info":
			var r GetDownloadInfoResp
			_, err := d.request("/samantha/aispace/get_download_info", http.MethodPost, func(req *resty.Request) {
				req.SetBody(base.Json{
					"requests": []base.Json{{"node_id": file.GetID()}},
				})
			}, &r)
			if err != nil {
				return nil, err
			}

			downloadUrl = r.Data.DownloadInfos[0].MainURL
		case "get_file_url":
			switch u.NodeType {
			case VideoType, AudioType:
				var r GetVideoFileUrlResp
				_, err := d.request("/samantha/media/get_play_info", http.MethodPost, func(req *resty.Request) {
					req.SetBody(base.Json{
						"key":     u.Key,
						"node_id": file.GetID(),
					})
				}, &r)
				if err != nil {
					return nil, err
				}

				downloadUrl = r.Data.OriginalMediaInfo.MainURL
			default:
				var r GetFileUrlResp
				_, err := d.request("/alice/message/get_file_url", http.MethodPost, func(req *resty.Request) {
					req.SetBody(base.Json{
						"uris": []string{u.Key},
						"type": FileNodeType[u.NodeType],
					})
				}, &r)
				if err != nil {
					return nil, err
				}

				downloadUrl = r.Data.FileUrls[0].MainURL
			}
		default:
			return nil, errs.NotImplement
		}

		// 生成标准的Content-Disposition
		contentDisposition := generateContentDisposition(u.Name)

		return &model.Link{
			URL: downloadUrl,
			Header: http.Header{
				"User-Agent":          []string{UserAgent},
				"Content-Disposition": []string{contentDisposition},
			},
		}, nil
	}

	return nil, errors.New("can't convert obj to URL")
}

func (d *Doubao) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	var r UploadNodeResp
	_, err := d.request("/samantha/aispace/upload_node", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"node_list": []base.Json{
				{
					"local_id":  uuid.New().String(),
					"name":      dirName,
					"parent_id": parentDir.GetID(),
					"node_type": 1,
				},
			},
		})
	}, &r)
	return err
}

func (d *Doubao) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	var r UploadNodeResp
	_, err := d.request("/samantha/aispace/move_node", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"node_list": []base.Json{
				{"id": srcObj.GetID()},
			},
			"current_parent_id": srcObj.GetPath(),
			"target_parent_id":  dstDir.GetID(),
		})
	}, &r)
	return err
}

func (d *Doubao) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	var r BaseResp
	_, err := d.request("/samantha/aispace/rename_node", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"node_id":   srcObj.GetID(),
			"node_name": newName,
		})
	}, &r)
	return err
}

func (d *Doubao) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO copy obj, optional
	return nil, errs.NotImplement
}

func (d *Doubao) Remove(ctx context.Context, obj model.Obj) error {
	var r BaseResp
	_, err := d.request("/samantha/aispace/delete_node", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{"node_list": []base.Json{{"id": obj.GetID()}}})
	}, &r)
	return err
}

func (d *Doubao) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// 根据MIME类型确定数据类型
	mimetype := file.GetMimetype()
	dataType := FileDataType

	switch {
	case strings.HasPrefix(mimetype, "video/"):
		dataType = VideoDataType
	case strings.HasPrefix(mimetype, "audio/"):
		dataType = VideoDataType // 音频与视频使用相同的处理方式
	case strings.HasPrefix(mimetype, "image/"):
		dataType = ImgDataType
	}

	// 获取上传配置
	uploadConfig := UploadConfig{}
	if err := d.getUploadConfig(&uploadConfig, dataType, file); err != nil {
		return nil, err
	}

	// 根据文件大小选择上传方式
	if file.GetSize() <= 1*utils.MB { // 小于1MB，使用普通模式上传
		return d.Upload(&uploadConfig, dstDir, file, up, dataType)
	}
	// 大文件使用分片上传
	return d.UploadByMultipart(ctx, &uploadConfig, file.GetSize(), dstDir, file, up, dataType)
}

func (d *Doubao) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Doubao) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Doubao) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Doubao) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *Doubao) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Doubao)(nil)
