package quqi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type Quqi struct {
	model.Storage
	Addition
	Cookie   string // Cookie
	GroupID  string // 私人云群组ID
	ClientID string // 随机生成客户端ID 经过测试，部分接口调用若不携带client id会出现错误
}

func (d *Quqi) Config() driver.Config {
	return config
}

func (d *Quqi) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Quqi) Init(ctx context.Context) error {
	// 登录
	if err := d.login(); err != nil {
		return err
	}

	// 生成随机client id (与网页端生成逻辑一致)
	d.ClientID = "quqipc_" + random.String(10)

	// 获取私人云ID (暂时仅获取私人云)
	groupResp := &GroupRes{}
	if _, err := d.request("group.quqi.com", "/v1/group/list", resty.MethodGet, nil, groupResp); err != nil {
		return err
	}
	for _, groupInfo := range groupResp.Data {
		if groupInfo == nil {
			continue
		}
		if groupInfo.Type == 2 {
			d.GroupID = strconv.Itoa(groupInfo.ID)
			break
		}
	}
	if d.GroupID == "" {
		return errs.StorageNotFound
	}

	return nil
}

func (d *Quqi) Drop(ctx context.Context) error {
	return nil
}

func (d *Quqi) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var (
		listResp = &ListRes{}
		files    []model.Obj
	)

	if _, err := d.request("", "/api/dir/ls", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"node_id":   dir.GetID(),
			"client_id": d.ClientID,
		})
	}, listResp); err != nil {
		return nil, err
	}

	if listResp.Data == nil {
		return nil, nil
	}

	// dirs
	for _, dirInfo := range listResp.Data.Dir {
		if dirInfo == nil {
			continue
		}
		files = append(files, &model.Object{
			ID:       strconv.FormatInt(dirInfo.NodeID, 10),
			Name:     dirInfo.Name,
			Modified: time.Unix(dirInfo.UpdateTime, 0),
			Ctime:    time.Unix(dirInfo.AddTime, 0),
			IsFolder: true,
		})
	}

	// files
	for _, fileInfo := range listResp.Data.File {
		if fileInfo == nil {
			continue
		}
		if fileInfo.EXT != "" {
			fileInfo.Name = strings.Join([]string{fileInfo.Name, fileInfo.EXT}, ".")
		}

		files = append(files, &model.Object{
			ID:       strconv.FormatInt(fileInfo.NodeID, 10),
			Name:     fileInfo.Name,
			Size:     fileInfo.Size,
			Modified: time.Unix(fileInfo.UpdateTime, 0),
			Ctime:    time.Unix(fileInfo.AddTime, 0),
		})
	}

	return files, nil
}

func (d *Quqi) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if d.CDN {
		link, err := d.linkFromCDN(file.GetID())
		if err != nil {
			log.Warn(err)
		} else {
			return link, nil
		}
	}

	link, err := d.linkFromPreview(file.GetID())
	if err != nil {
		log.Warn(err)
	} else {
		return link, nil
	}

	link, err = d.linkFromDownload(file.GetID())
	if err != nil {
		return nil, err
	}
	return link, nil
}

func (d *Quqi) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	var (
		makeDirRes = &MakeDirRes{}
		timeNow    = time.Now()
	)

	if _, err := d.request("", "/api/dir/mkDir", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"parent_id": parentDir.GetID(),
			"name":      dirName,
			"client_id": d.ClientID,
		})
	}, makeDirRes); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       strconv.FormatInt(makeDirRes.Data.NodeID, 10),
		Name:     dirName,
		Modified: timeNow,
		Ctime:    timeNow,
		IsFolder: true,
	}, nil
}

func (d *Quqi) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var moveRes = &MoveRes{}

	if _, err := d.request("", "/api/dir/mvDir", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":        d.GroupID,
			"tree_id":        "1",
			"node_id":        dstDir.GetID(),
			"source_quqi_id": d.GroupID,
			"source_tree_id": "1",
			"source_node_id": srcObj.GetID(),
			"client_id":      d.ClientID,
		})
	}, moveRes); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       strconv.FormatInt(moveRes.Data.NodeID, 10),
		Name:     moveRes.Data.NodeName,
		Size:     srcObj.GetSize(),
		Modified: time.Now(),
		Ctime:    srcObj.CreateTime(),
		IsFolder: srcObj.IsDir(),
	}, nil
}

func (d *Quqi) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	var realName = newName

	if !srcObj.IsDir() {
		srcExt, newExt := utils.Ext(srcObj.GetName()), utils.Ext(newName)

		// 曲奇网盘的文件名称由文件名和扩展名组成，若存在扩展名，则重命名时仅支持更改文件名，扩展名在曲奇服务端保留
		if srcExt != "" && srcExt == newExt {
			parts := strings.Split(newName, ".")
			if len(parts) > 1 {
				realName = strings.Join(parts[:len(parts)-1], ".")
			}
		}
	}

	if _, err := d.request("", "/api/dir/renameDir", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"node_id":   srcObj.GetID(),
			"rename":    realName,
			"client_id": d.ClientID,
		})
	}, nil); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       srcObj.GetID(),
		Name:     newName,
		Size:     srcObj.GetSize(),
		Modified: time.Now(),
		Ctime:    srcObj.CreateTime(),
		IsFolder: srcObj.IsDir(),
	}, nil
}

func (d *Quqi) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// 无法从曲奇接口响应中直接获取复制后的文件信息
	if _, err := d.request("", "/api/node/copy", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":        d.GroupID,
			"tree_id":        "1",
			"node_id":        dstDir.GetID(),
			"source_quqi_id": d.GroupID,
			"source_tree_id": "1",
			"source_node_id": srcObj.GetID(),
			"client_id":      d.ClientID,
		})
	}, nil); err != nil {
		return nil, err
	}

	return nil, nil
}

func (d *Quqi) Remove(ctx context.Context, obj model.Obj) error {
	// 暂时不做直接删除，默认都放到回收站。直接删除方法：先调用删除接口放入回收站，在通过回收站接口删除文件
	if _, err := d.request("", "/api/node/del", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"node_id":   obj.GetID(),
			"client_id": d.ClientID,
		})
	}, nil); err != nil {
		return err
	}

	return nil
}

func (d *Quqi) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// base info
	sizeStr := strconv.FormatInt(stream.GetSize(), 10)
	f, err := stream.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}
	md5, err := utils.HashFile(utils.MD5, f)
	if err != nil {
		return nil, err
	}
	sha, err := utils.HashFile(utils.SHA256, f)
	if err != nil {
		return nil, err
	}
	// init upload
	var uploadInitResp UploadInitResp
	_, err = d.request("", "/api/upload/v1/file/init", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"parent_id": dstDir.GetID(),
			"size":      sizeStr,
			"file_name": stream.GetName(),
			"md5":       md5,
			"sha":       sha,
			"is_slice":  "true",
			"client_id": d.ClientID,
		})
	}, &uploadInitResp)
	if err != nil {
		return nil, err
	}
	// check exist
	// if the file already exists in Quqi server, there is no need to actually upload it
	if uploadInitResp.Data.Exist {
		// the file name returned by Quqi does not include the extension name
		nodeName, nodeExt := uploadInitResp.Data.NodeName, utils.Ext(stream.GetName())
		if nodeExt != "" {
			nodeName = nodeName + "." + nodeExt
		}
		return &model.Object{
			ID:       strconv.FormatInt(uploadInitResp.Data.NodeID, 10),
			Name:     nodeName,
			Size:     stream.GetSize(),
			Modified: stream.ModTime(),
			Ctime:    stream.CreateTime(),
		}, nil
	}
	// listParts
	_, err = d.request("upload.quqi.com:20807", "/upload/v1/listParts", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"token":     uploadInitResp.Data.Token,
			"task_id":   uploadInitResp.Data.TaskID,
			"client_id": d.ClientID,
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	// get temp key
	var tempKeyResp TempKeyResp
	_, err = d.request("upload.quqi.com:20807", "/upload/v1/tempKey", resty.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"token":   uploadInitResp.Data.Token,
			"task_id": uploadInitResp.Data.TaskID,
		})
	}, &tempKeyResp)
	if err != nil {
		return nil, err
	}
	// upload
	// u, err := url.Parse(fmt.Sprintf("https://%s.cos.ap-shanghai.myqcloud.com", uploadInitResp.Data.Bucket))
	// b := &cos.BaseURL{BucketURL: u}
	// client := cos.NewClient(b, &http.Client{
	// 	Transport: &cos.CredentialTransport{
	// 		Credential: cos.NewTokenCredential(tempKeyResp.Data.Credentials.TmpSecretID, tempKeyResp.Data.Credentials.TmpSecretKey, tempKeyResp.Data.Credentials.SessionToken),
	// 	},
	// })
	// partSize := int64(1024 * 1024 * 2)
	// partCount := (stream.GetSize() + partSize - 1) / partSize
	// for i := 1; i <= int(partCount); i++ {
	// 	length := partSize
	// 	if i == int(partCount) {
	// 		length = stream.GetSize() - (int64(i)-1)*partSize
	// 	}
	// 	_, err := client.Object.UploadPart(
	// 		ctx, uploadInitResp.Data.Key, uploadInitResp.Data.UploadID, i, io.LimitReader(f, partSize), &cos.ObjectUploadPartOptions{
	// 			ContentLength: length,
	// 		},
	// 	)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	cfg := &aws.Config{
		Credentials: credentials.NewStaticCredentials(tempKeyResp.Data.Credentials.TmpSecretID, tempKeyResp.Data.Credentials.TmpSecretKey, tempKeyResp.Data.Credentials.SessionToken),
		Region:      aws.String("ap-shanghai"),
		Endpoint:    aws.String("cos.ap-shanghai.myqcloud.com"),
	}
	s, err := session.NewSession(cfg)
	if err != nil {
		return nil, err
	}
	uploader := s3manager.NewUploader(s)
	buf := make([]byte, 1024*1024*2)
	fup := &driver.ReaderUpdatingProgress{
		Reader: &driver.SimpleReaderWithSize{
			Reader: f,
			Size:   int64(len(buf)),
		},
		UpdateProgress: up,
	}
	for partNumber := int64(1); ; partNumber++ {
		n, err := io.ReadFull(fup, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		reader := bytes.NewReader(buf[:n])
		_, err = uploader.S3.UploadPartWithContext(ctx, &s3.UploadPartInput{
			UploadId:   &uploadInitResp.Data.UploadID,
			Key:        &uploadInitResp.Data.Key,
			Bucket:     &uploadInitResp.Data.Bucket,
			PartNumber: aws.Int64(partNumber),
			Body: struct {
				*driver.RateLimitReader
				io.Seeker
			}{
				RateLimitReader: driver.NewLimitedUploadStream(ctx, reader),
				Seeker:          reader,
			},
		})
		if err != nil {
			return nil, err
		}
	}
	// finish upload
	var uploadFinishResp UploadFinishResp
	_, err = d.request("", "/api/upload/v1/file/finish", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"token":     uploadInitResp.Data.Token,
			"task_id":   uploadInitResp.Data.TaskID,
			"client_id": d.ClientID,
		})
	}, &uploadFinishResp)
	if err != nil {
		return nil, err
	}
	// the file name returned by Quqi does not include the extension name
	nodeName, nodeExt := uploadFinishResp.Data.NodeName, utils.Ext(stream.GetName())
	if nodeExt != "" {
		nodeName = nodeName + "." + nodeExt
	}
	return &model.Object{
		ID:       strconv.FormatInt(uploadFinishResp.Data.NodeID, 10),
		Name:     nodeName,
		Size:     stream.GetSize(),
		Modified: stream.ModTime(),
		Ctime:    stream.CreateTime(),
	}, nil
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Quqi)(nil)
