package aliyundrive_open

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Xhofe/rateg"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

type AliyundriveOpen struct {
	model.Storage
	Addition

	DriveId string

	limitList func(ctx context.Context, data base.Json) (*Files, error)
	limitLink func(ctx context.Context, file model.Obj) (*model.Link, error)
	ref       *AliyundriveOpen
}

func (d *AliyundriveOpen) Config() driver.Config {
	return config
}

func (d *AliyundriveOpen) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *AliyundriveOpen) Init(ctx context.Context) error {
	if d.LIVPDownloadFormat == "" {
		d.LIVPDownloadFormat = "jpeg"
	}
	if d.DriveType == "" {
		d.DriveType = "default"
	}
	res, err := d.request("/adrive/v1.0/user/getDriveInfo", http.MethodPost, nil)
	if err != nil {
		return err
	}
	d.DriveId = utils.Json.Get(res, d.DriveType+"_drive_id").ToString()
	d.limitList = rateg.LimitFnCtx(d.list, rateg.LimitFnOption{
		Limit:  4,
		Bucket: 1,
	})
	d.limitLink = rateg.LimitFnCtx(d.link, rateg.LimitFnOption{
		Limit:  1,
		Bucket: 1,
	})
	return nil
}

func (d *AliyundriveOpen) InitReference(storage driver.Driver) error {
	refStorage, ok := storage.(*AliyundriveOpen)
	if ok {
		d.ref = refStorage
		return nil
	}
	return errs.NotSupport
}

func (d *AliyundriveOpen) Drop(ctx context.Context) error {
	d.ref = nil
	return nil
}

// GetRoot implements the driver.GetRooter interface to properly set up the root object
func (d *AliyundriveOpen) GetRoot(ctx context.Context) (model.Obj, error) {
	return &model.Object{
		ID:       d.RootFolderID,
		Path:     "/",
		Name:     "root",
		Size:     0,
		Modified: d.Modified,
		IsFolder: true,
	}, nil
}

func (d *AliyundriveOpen) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if d.limitList == nil {
		return nil, fmt.Errorf("driver not init")
	}
	files, err := d.getFiles(ctx, dir.GetID())
	if err != nil {
		return nil, err
	}

	objs, err := utils.SliceConvert(files, func(src File) (model.Obj, error) {
		obj := fileToObj(src)
		// Set the correct path for the object
		if dir.GetPath() != "" {
			obj.Path = filepath.Join(dir.GetPath(), obj.GetName())
		}
		return obj, nil
	})

	return objs, err
}

func (d *AliyundriveOpen) link(ctx context.Context, file model.Obj) (*model.Link, error) {
	res, err := d.request("/adrive/v1.0/openFile/getDownloadUrl", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":   d.DriveId,
			"file_id":    file.GetID(),
			"expire_sec": 14400,
		})
	})
	if err != nil {
		return nil, err
	}
	url := utils.Json.Get(res, "url").ToString()
	if url == "" {
		if utils.Ext(file.GetName()) != "livp" {
			return nil, errors.New("get download url failed: " + string(res))
		}
		url = utils.Json.Get(res, "streamsUrl", d.LIVPDownloadFormat).ToString()
	}
	exp := time.Minute
	return &model.Link{
		URL:        url,
		Expiration: &exp,
	}, nil
}

func (d *AliyundriveOpen) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if d.limitLink == nil {
		return nil, fmt.Errorf("driver not init")
	}
	return d.limitLink(ctx, file)
}

func (d *AliyundriveOpen) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	nowTime, _ := getNowTime()
	newDir := File{CreatedAt: nowTime, UpdatedAt: nowTime}
	_, err := d.request("/adrive/v1.0/openFile/create", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":        d.DriveId,
			"parent_file_id":  parentDir.GetID(),
			"name":            dirName,
			"type":            "folder",
			"check_name_mode": "refuse",
		}).SetResult(&newDir)
	})
	if err != nil {
		return nil, err
	}
	obj := fileToObj(newDir)

	// Set the correct Path for the returned directory object
	if parentDir.GetPath() != "" {
		obj.Path = filepath.Join(parentDir.GetPath(), dirName)
	} else {
		obj.Path = "/" + dirName
	}

	return obj, nil
}

func (d *AliyundriveOpen) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var resp MoveOrCopyResp
	_, err := d.request("/adrive/v1.0/openFile/move", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":          d.DriveId,
			"file_id":           srcObj.GetID(),
			"to_parent_file_id": dstDir.GetID(),
			"check_name_mode":   "ignore", // optional:ignore,auto_rename,refuse
			//"new_name":          "newName", // The new name to use when a file of the same name exists
		}).SetResult(&resp)
	})
	if err != nil {
		return nil, err
	}

	if srcObj, ok := srcObj.(*model.ObjThumb); ok {
		srcObj.ID = resp.FileID
		srcObj.Modified = time.Now()
		srcObj.Path = filepath.Join(dstDir.GetPath(), srcObj.GetName())

		// Check for duplicate files in the destination directory
		if err := d.removeDuplicateFiles(ctx, dstDir.GetPath(), srcObj.GetName(), srcObj.GetID()); err != nil {
			// Only log a warning instead of returning an error since the move operation has already completed successfully
			log.Warnf("Failed to remove duplicate files after move: %v", err)
		}
		return srcObj, nil
	}
	return nil, nil
}

func (d *AliyundriveOpen) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	var newFile File
	_, err := d.request("/adrive/v1.0/openFile/update", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id": d.DriveId,
			"file_id":  srcObj.GetID(),
			"name":     newName,
		}).SetResult(&newFile)
	})
	if err != nil {
		return nil, err
	}

	// Check for duplicate files in the parent directory
	parentPath := filepath.Dir(srcObj.GetPath())
	if err := d.removeDuplicateFiles(ctx, parentPath, newName, newFile.FileId); err != nil {
		// Only log a warning instead of returning an error since the rename operation has already completed successfully
		log.Warnf("Failed to remove duplicate files after rename: %v", err)
	}

	obj := fileToObj(newFile)

	// Set the correct Path for the renamed object
	if parentPath != "" && parentPath != "." {
		obj.Path = filepath.Join(parentPath, newName)
	} else {
		obj.Path = "/" + newName
	}

	return obj, nil
}

func (d *AliyundriveOpen) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	var resp MoveOrCopyResp
	_, err := d.request("/adrive/v1.0/openFile/copy", http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id":          d.DriveId,
			"file_id":           srcObj.GetID(),
			"to_parent_file_id": dstDir.GetID(),
			"auto_rename":       false,
		}).SetResult(&resp)
	})
	if err != nil {
		return err
	}

	// Check for duplicate files in the destination directory
	if err := d.removeDuplicateFiles(ctx, dstDir.GetPath(), srcObj.GetName(), resp.FileID); err != nil {
		// Only log a warning instead of returning an error since the copy operation has already completed successfully
		log.Warnf("Failed to remove duplicate files after copy: %v", err)
	}

	return nil
}

func (d *AliyundriveOpen) Remove(ctx context.Context, obj model.Obj) error {
	uri := "/adrive/v1.0/openFile/recyclebin/trash"
	if d.RemoveWay == "delete" {
		uri = "/adrive/v1.0/openFile/delete"
	}
	_, err := d.request(uri, http.MethodPost, func(req *resty.Request) {
		req.SetBody(base.Json{
			"drive_id": d.DriveId,
			"file_id":  obj.GetID(),
		})
	})
	return err
}

func (d *AliyundriveOpen) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	obj, err := d.upload(ctx, dstDir, stream, up)

	// Set the correct Path for the returned file object
	if obj != nil && obj.GetPath() == "" {
		if dstDir.GetPath() != "" {
			if objWithPath, ok := obj.(model.SetPath); ok {
				objWithPath.SetPath(filepath.Join(dstDir.GetPath(), obj.GetName()))
			}
		}
	}

	return obj, err
}

func (d *AliyundriveOpen) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	var resp base.Json
	var uri string
	data := base.Json{
		"drive_id": d.DriveId,
		"file_id":  args.Obj.GetID(),
	}
	switch args.Method {
	case "video_preview":
		uri = "/adrive/v1.0/openFile/getVideoPreviewPlayInfo"
		data["category"] = "live_transcoding"
		data["url_expire_sec"] = 14400
	default:
		return nil, errs.NotSupport
	}
	_, err := d.request(uri, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data).SetResult(&resp)
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

var _ driver.Driver = (*AliyundriveOpen)(nil)
var _ driver.MkdirResult = (*AliyundriveOpen)(nil)
var _ driver.MoveResult = (*AliyundriveOpen)(nil)
var _ driver.RenameResult = (*AliyundriveOpen)(nil)
var _ driver.PutResult = (*AliyundriveOpen)(nil)
var _ driver.GetRooter = (*AliyundriveOpen)(nil)
