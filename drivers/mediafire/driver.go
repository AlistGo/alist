package mediafire

/*
Package mediafire
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-11

D@' 3z K!7 - The King Of Cracking
*/

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/cron"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type Mediafire struct {
	model.Storage
	Addition
	cron *cron.Cron

	actionToken string

	appBase    string
	apiBase    string
	hostBase   string
	maxRetries int

	secChUa         string
	secChUaPlatform string
	userAgent       string
}

func (d *Mediafire) Config() driver.Config {
	return config
}

func (d *Mediafire) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Mediafire) Init(ctx context.Context) error {
	if d.SessionToken == "" {
		return fmt.Errorf("Init :: [MediaFire] {critical} missing sessionToken")
	}

	if d.Cookie == "" {
		return fmt.Errorf("Init :: [MediaFire] {critical} missing Cookie")
	}

	if _, err := d.getSessionToken(ctx); err != nil {

		d.renewToken(ctx)

		num := rand.Intn(4) + 6

		d.cron = cron.NewCron(time.Minute * time.Duration(num))
		d.cron.Do(func() {
			d.renewToken(ctx)
		})

	}

	return nil
}

func (d *Mediafire) Drop(ctx context.Context) error {
	return nil
}

func (d *Mediafire) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(ctx, dir.GetID())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return d.fileToObj(src), nil
	})
}

func (d *Mediafire) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {

	downloadUrl, err := d.getDirectDownloadLink(ctx, file.GetID())
	if err != nil {
		return nil, err
	}

	res, err := base.NoRedirectClient.R().SetDoNotParseResponse(true).SetContext(ctx).Get(downloadUrl)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = res.RawBody().Close()
	}()

	if res.StatusCode() == 302 {
		downloadUrl = res.Header().Get("location")
	}

	return &model.Link{
		URL: downloadUrl,
		Header: http.Header{
			"Origin":             []string{d.appBase},
			"Referer":            []string{d.appBase + "/"},
			"sec-ch-ua":          []string{d.secChUa},
			"sec-ch-ua-platform": []string{d.secChUaPlatform},
			"User-Agent":         []string{d.userAgent},
			//"User-Agent": []string{base.UserAgent},
		},
	}, nil
}

func (d *Mediafire) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	data := map[string]string{
		"session_token":   d.SessionToken,
		"response_format": "json",
		"parent_key":      parentDir.GetID(),
		"foldername":      dirName,
	}

	var resp MediafireFolderCreateResponse
	_, err := d.postForm("/folder/create.php", data, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	created, _ := time.Parse("2006-01-02T15:04:05Z", resp.Response.CreatedUTC)

	return &model.ObjThumb{
		Object: model.Object{
			ID:       resp.Response.FolderKey,
			Name:     resp.Response.Name,
			Size:     0,
			Modified: created,
			Ctime:    created,
			IsFolder: true,
		},
		Thumbnail: model.Thumbnail{},
	}, nil
}

func (d *Mediafire) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var data map[string]string
	var endpoint string

	if srcObj.IsDir() {

		endpoint = "/folder/move.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"folder_key_src":  srcObj.GetID(),
			"folder_key_dst":  dstDir.GetID(),
		}
	} else {

		endpoint = "/file/move.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"quick_key":       srcObj.GetID(),
			"folder_key":      dstDir.GetID(),
		}
	}

	var resp MediafireMoveResponse
	_, err := d.postForm(endpoint, data, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	return srcObj, nil
}

func (d *Mediafire) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	var data map[string]string
	var endpoint string

	if srcObj.IsDir() {

		endpoint = "/folder/update.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"folder_key":      srcObj.GetID(),
			"foldername":      newName,
		}
	} else {

		endpoint = "/file/update.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"quick_key":       srcObj.GetID(),
			"filename":        newName,
		}
	}

	var resp MediafireRenameResponse
	_, err := d.postForm(endpoint, data, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	return &model.ObjThumb{
		Object: model.Object{
			ID:       srcObj.GetID(),
			Name:     newName,
			Size:     srcObj.GetSize(),
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
			IsFolder: srcObj.IsDir(),
		},
		Thumbnail: model.Thumbnail{},
	}, nil
}

func (d *Mediafire) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var data map[string]string
	var endpoint string

	if srcObj.IsDir() {

		endpoint = "/folder/copy.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"folder_key_src":  srcObj.GetID(),
			"folder_key_dst":  dstDir.GetID(),
		}
	} else {

		endpoint = "/file/copy.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"quick_key":       srcObj.GetID(),
			"folder_key":      dstDir.GetID(),
		}
	}

	var resp MediafireCopyResponse
	_, err := d.postForm(endpoint, data, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	var newID string
	if srcObj.IsDir() {
		if len(resp.Response.NewFolderKeys) > 0 {
			newID = resp.Response.NewFolderKeys[0]
		}
	} else {
		if len(resp.Response.NewQuickKeys) > 0 {
			newID = resp.Response.NewQuickKeys[0]
		}
	}

	return &model.ObjThumb{
		Object: model.Object{
			ID:       newID,
			Name:     srcObj.GetName(),
			Size:     srcObj.GetSize(),
			Modified: srcObj.ModTime(),
			Ctime:    srcObj.CreateTime(),
			IsFolder: srcObj.IsDir(),
		},
		Thumbnail: model.Thumbnail{},
	}, nil
}

func (d *Mediafire) Remove(ctx context.Context, obj model.Obj) error {
	var data map[string]string
	var endpoint string

	if obj.IsDir() {

		endpoint = "/folder/delete.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"folder_key":      obj.GetID(),
		}
	} else {

		endpoint = "/file/delete.php"
		data = map[string]string{
			"session_token":   d.SessionToken,
			"response_format": "json",
			"quick_key":       obj.GetID(),
		}
	}

	var resp MediafireRemoveResponse
	_, err := d.postForm(endpoint, data, &resp)
	if err != nil {
		return err
	}

	if resp.Response.Result != "Success" {
		return fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	return nil
}

func (d *Mediafire) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	_, err := d.PutResult(ctx, dstDir, file, up)
	return err
}

func (d *Mediafire) PutResult(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {

	tempFile, err := file.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}
	defer tempFile.Close()

	osFile, ok := tempFile.(*os.File)
	if !ok {
		return nil, fmt.Errorf("expected *os.File, got %T", tempFile)
	}

	fileHash, err := d.calculateSHA256(osFile)
	if err != nil {
		return nil, err
	}

	checkResp, err := d.uploadCheck(ctx, file.GetName(), file.GetSize(), fileHash, dstDir.GetID())
	if err != nil {
		return nil, err
	}

	if checkResp.Response.ResumableUpload.AllUnitsReady == "yes" {
		up(100.0)
	}

	if checkResp.Response.HashExists == "yes" && checkResp.Response.InAccount == "yes" {
		up(100.0)
		existingFile, err := d.getExistingFileInfo(ctx, fileHash, file.GetName(), dstDir.GetID())
		if err == nil {
			return existingFile, nil
		}
	}

	var pollKey string

	if checkResp.Response.ResumableUpload.AllUnitsReady != "yes" {

		var err error

		pollKey, err = d.uploadUnits(ctx, osFile, checkResp, file.GetName(), fileHash, dstDir.GetID(), up)
		if err != nil {
			return nil, err
		}
	} else {

		pollKey = checkResp.Response.ResumableUpload.UploadKey
	}

	//fmt.Printf("pollKey: %+v\n", pollKey)

	pollResp, err := d.pollUpload(ctx, pollKey)
	if err != nil {
		return nil, err
	}

	quickKey := pollResp.Response.Doupload.QuickKey

	return &model.ObjThumb{
		Object: model.Object{
			ID:   quickKey,
			Name: file.GetName(),
			Size: file.GetSize(),
		},
		Thumbnail: model.Thumbnail{},
	}, nil
}

func (d *Mediafire) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Mediafire) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Mediafire) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *Mediafire) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

//func (d *Mediafire) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Mediafire)(nil)
