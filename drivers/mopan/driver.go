package mopan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/errgroup"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/foxxorcat/mopan-sdk-go"
	log "github.com/sirupsen/logrus"
)

type MoPan struct {
	model.Storage
	Addition
	client *mopan.MoClient

	userID       string
	uploadThread int
}

func (d *MoPan) Config() driver.Config {
	return config
}

func (d *MoPan) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *MoPan) Init(ctx context.Context) error {
	d.uploadThread, _ = strconv.Atoi(d.UploadThread)
	if d.uploadThread < 1 || d.uploadThread > 32 {
		d.uploadThread, d.UploadThread = 3, "3"
	}

	defer func() { d.SMSCode = "" }()

	login := func() (err error) {
		var loginData *mopan.LoginResp
		if d.SMSCode != "" {
			loginData, err = d.client.LoginBySmsStep2(d.Phone, d.SMSCode)
		} else {
			loginData, err = d.client.Login(d.Phone, d.Password)
		}
		if err != nil {
			return err
		}
		d.client.SetAuthorization(loginData.Token)

		info, err := d.client.GetUserInfo()
		if err != nil {
			return err
		}
		d.userID = info.UserID
		log.Debugf("[mopan] Phone: %s UserCloudStorageRelations: %+v", d.Phone, loginData.UserCloudStorageRelations)
		cloudCircleApp, _ := d.client.QueryAllCloudCircleApp()
		log.Debugf("[mopan] Phone: %s CloudCircleApp: %+v", d.Phone, cloudCircleApp)
		if d.RootFolderID == "" {
			for _, userCloudStorage := range loginData.UserCloudStorageRelations {
				if userCloudStorage.Path == "/文件" {
					d.RootFolderID = userCloudStorage.FolderID
				}
			}
		}
		return nil
	}
	d.client = mopan.NewMoClientWithRestyClient(base.NewRestyClient()).
		SetRestyClient(base.RestyClient).
		SetOnAuthorizationExpired(func(_ error) error {
			err := login()
			if err != nil {
				d.Status = err.Error()
				op.MustSaveDriverStorage(d)
			}
			return err
		})

	var deviceInfo mopan.DeviceInfo
	if strings.TrimSpace(d.DeviceInfo) != "" && utils.Json.UnmarshalFromString(d.DeviceInfo, &deviceInfo) == nil {
		d.client.SetDeviceInfo(&deviceInfo)
	}
	d.DeviceInfo, _ = utils.Json.MarshalToString(d.client.GetDeviceInfo())

	if strings.Contains(d.SMSCode, "send") {
		if _, err := d.client.LoginBySms(d.Phone); err != nil {
			return err
		}
		return errors.New("please enter the SMS code")
	}
	return login()
}

func (d *MoPan) Drop(ctx context.Context) error {
	d.client = nil
	d.userID = ""
	return nil
}

func (d *MoPan) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var files []model.Obj
	for page := 1; ; page++ {
		data, err := d.client.QueryFiles(dir.GetID(), page, mopan.WarpParamOption(
			func(j mopan.Json) {
				j["orderBy"] = d.OrderBy
				j["descending"] = d.OrderDirection == "desc"
			},
			mopan.ParamOptionShareFile(d.CloudID),
		))
		if err != nil {
			return nil, err
		}

		if len(data.FileListAO.FileList)+len(data.FileListAO.FolderList) == 0 {
			break
		}

		log.Debugf("[mopan] Phone: %s folder: %+v", d.Phone, data.FileListAO.FolderList)
		files = append(files, utils.MustSliceConvert(data.FileListAO.FolderList, folderToObj)...)
		files = append(files, utils.MustSliceConvert(data.FileListAO.FileList, fileToObj)...)
	}
	return files, nil
}

func (d *MoPan) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	data, err := d.client.GetFileDownloadUrl(file.GetID(), mopan.WarpParamOption(mopan.ParamOptionShareFile(d.CloudID)))
	if err != nil {
		return nil, err
	}

	data.DownloadUrl = strings.Replace(strings.ReplaceAll(data.DownloadUrl, "&amp;", "&"), "http://", "https://", 1)
	res, err := base.NoRedirectClient.R().SetDoNotParseResponse(true).SetContext(ctx).Get(data.DownloadUrl)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = res.RawBody().Close()
	}()
	if res.StatusCode() == 302 {
		data.DownloadUrl = res.Header().Get("location")
	}

	return &model.Link{
		URL: data.DownloadUrl,
	}, nil
}

func (d *MoPan) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	f, err := d.client.CreateFolder(dirName, parentDir.GetID(), mopan.WarpParamOption(
		mopan.ParamOptionShareFile(d.CloudID),
	))
	if err != nil {
		return nil, err
	}
	return folderToObj(*f), nil
}

func (d *MoPan) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.newTask(srcObj, dstDir, mopan.TASK_MOVE)
}

func (d *MoPan) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if srcObj.IsDir() {
		_, err := d.client.RenameFolder(srcObj.GetID(), newName, mopan.WarpParamOption(
			mopan.ParamOptionShareFile(d.CloudID),
		))
		if err != nil {
			return nil, err
		}
	} else {
		_, err := d.client.RenameFile(srcObj.GetID(), newName, mopan.WarpParamOption(
			mopan.ParamOptionShareFile(d.CloudID),
		))
		if err != nil {
			return nil, err
		}
	}
	return CloneObj(srcObj, srcObj.GetID(), newName), nil
}

func (d *MoPan) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.newTask(srcObj, dstDir, mopan.TASK_COPY)
}

func (d *MoPan) newTask(srcObj, dstDir model.Obj, taskType mopan.TaskType) (model.Obj, error) {
	param := mopan.TaskParam{
		UserOrCloudID:       d.userID,
		Source:              1,
		TaskType:            taskType,
		TargetSource:        1,
		TargetUserOrCloudID: d.userID,
		TargetType:          1,
		TargetFolderID:      dstDir.GetID(),
		TaskStatusDetailDTOList: []mopan.TaskFileParam{
			{
				FileID:   srcObj.GetID(),
				IsFolder: srcObj.IsDir(),
				FileName: srcObj.GetName(),
			},
		},
	}
	if d.CloudID != "" {
		param.UserOrCloudID = d.CloudID
		param.Source = 2
		param.TargetSource = 2
		param.TargetUserOrCloudID = d.CloudID
	}

	task, err := d.client.AddBatchTask(param)
	if err != nil {
		return nil, err
	}

	for count := 0; count < 5; count++ {
		stat, err := d.client.CheckBatchTask(mopan.TaskCheckParam{
			TaskId:              task.TaskIDList[0],
			TaskType:            task.TaskType,
			TargetType:          1,
			TargetFolderID:      task.TargetFolderID,
			TargetSource:        param.TargetSource,
			TargetUserOrCloudID: param.TargetUserOrCloudID,
		})
		if err != nil {
			return nil, err
		}

		switch stat.TaskStatus {
		case 2:
			if err := d.client.CancelBatchTask(stat.TaskID, task.TaskType); err != nil {
				return nil, err
			}
			return nil, errors.New("file name conflict")
		case 4:
			if task.TaskType == mopan.TASK_MOVE {
				return CloneObj(srcObj, srcObj.GetID(), srcObj.GetName()), nil
			}
			return CloneObj(srcObj, stat.SuccessedFileIDList[0], srcObj.GetName()), nil
		}
		time.Sleep(time.Second)
	}
	return nil, nil
}

func (d *MoPan) Remove(ctx context.Context, obj model.Obj) error {
	_, err := d.client.DeleteToRecycle([]mopan.TaskFileParam{
		{
			FileID:   obj.GetID(),
			IsFolder: obj.IsDir(),
			FileName: obj.GetName(),
		},
	}, mopan.WarpParamOption(mopan.ParamOptionShareFile(d.CloudID)))
	return err
}

func (d *MoPan) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	file, err := stream.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}

	// step.1
	uploadPartData, err := mopan.InitUploadPartData(ctx, mopan.UpdloadFileParam{
		ParentFolderId: dstDir.GetID(),
		FileName:       stream.GetName(),
		FileSize:       stream.GetSize(),
		File:           file,
	})
	if err != nil {
		return nil, err
	}

	// 尝试恢复进度
	initUpdload, ok := base.GetUploadProgress[*mopan.InitMultiUploadData](d, d.client.Authorization, uploadPartData.FileMd5)
	if !ok {
		// step.2
		initUpdload, err = d.client.InitMultiUpload(ctx, *uploadPartData, mopan.WarpParamOption(
			mopan.ParamOptionShareFile(d.CloudID),
		))
		if err != nil {
			return nil, err
		}
	}

	if !initUpdload.FileDataExists {
		// utils.Log.Error(d.client.CloudDiskStartBusiness())

		threadG, upCtx := errgroup.NewGroupWithContext(ctx, d.uploadThread,
			retry.Attempts(3),
			retry.Delay(time.Second),
			retry.DelayType(retry.BackOffDelay))
		sem := semaphore.NewWeighted(3)

		// step.3
		parts, err := d.client.GetAllMultiUploadUrls(initUpdload.UploadFileID, initUpdload.PartInfos)
		if err != nil {
			return nil, err
		}

		for i, part := range parts {
			if utils.IsCanceled(upCtx) {
				break
			}
			i, part, byteSize := i, part, initUpdload.PartSize
			if part.PartNumber == uploadPartData.PartTotal {
				byteSize = initUpdload.LastPartSize
			}

			// step.4
			threadG.Go(func(ctx context.Context) error {
				if err = sem.Acquire(ctx, 1); err != nil {
					return err
				}
				defer sem.Release(1)
				reader := io.NewSectionReader(file, int64(part.PartNumber-1)*initUpdload.PartSize, byteSize)
				req, err := part.NewRequest(ctx, driver.NewLimitedUploadStream(ctx, reader))
				if err != nil {
					return err
				}
				req.ContentLength = byteSize
				resp, err := base.HttpClient.Do(req)
				if err != nil {
					return err
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("upload err,code=%d", resp.StatusCode)
				}
				up(100 * float64(threadG.Success()) / float64(len(parts)))
				initUpdload.PartInfos[i] = ""
				return nil
			})
		}
		if err = threadG.Wait(); err != nil {
			if errors.Is(err, context.Canceled) {
				initUpdload.PartInfos = utils.SliceFilter(initUpdload.PartInfos, func(s string) bool { return s != "" })
				base.SaveUploadProgress(d, initUpdload, d.client.Authorization, uploadPartData.FileMd5)
			}
			return nil, err
		}
	}
	//step.5
	uFile, err := d.client.CommitMultiUploadFile(initUpdload.UploadFileID, nil)
	if err != nil {
		return nil, err
	}
	return &model.Object{
		ID:       uFile.UserFileID,
		Name:     uFile.FileName,
		Size:     int64(uFile.FileSize),
		Modified: time.Time(uFile.CreateDate),
	}, nil
}

var _ driver.Driver = (*MoPan)(nil)
var _ driver.MkdirResult = (*MoPan)(nil)
var _ driver.MoveResult = (*MoPan)(nil)
var _ driver.RenameResult = (*MoPan)(nil)
var _ driver.Remove = (*MoPan)(nil)
var _ driver.CopyResult = (*MoPan)(nil)
var _ driver.PutResult = (*MoPan)(nil)
