package pcloud

import (
	"context"
	"fmt"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

type PCloud struct {
	model.Storage
	Addition
	AccessToken string // Actual access token obtained from refresh token
}

func (d *PCloud) Config() driver.Config {
	return config
}

func (d *PCloud) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *PCloud) Init(ctx context.Context) error {
	// Map hostname selection to actual API endpoints
	if d.Hostname == "us" {
		d.Hostname = "api.pcloud.com"
	} else if d.Hostname == "eu" {
		d.Hostname = "eapi.pcloud.com"
	}

	// Set default root folder ID if not provided
	if d.RootFolderID == "" {
		d.RootFolderID = "d0"
	}

	// Use the access token directly (like rclone)
	d.AccessToken = d.RefreshToken // RefreshToken field actually contains the access_token
	return nil
}

func (d *PCloud) Drop(ctx context.Context) error {
	return nil
}

func (d *PCloud) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	folderID := d.RootFolderID
	if dir.GetID() != "" {
		folderID = dir.GetID()
	}

	files, err := d.getFiles(folderID)
	if err != nil {
		return nil, err
	}

	return utils.SliceConvert(files, func(src FileObject) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *PCloud) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	downloadURL, err := d.getDownloadLink(file.GetID())
	if err != nil {
		return nil, err
	}

	return &model.Link{
		URL: downloadURL,
	}, nil
}

// Mkdir implements driver.Mkdir
func (d *PCloud) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	parentID := d.RootFolderID
	if parentDir.GetID() != "" {
		parentID = parentDir.GetID()
	}

	return d.createFolder(parentID, dirName)
}

// Move implements driver.Move
func (d *PCloud) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	// pCloud uses renamefile/renamefolder for both rename and move
	endpoint := "/renamefile"
	paramName := "fileid"

	if srcObj.IsDir() {
		endpoint = "/renamefolder"
		paramName = "folderid"
	}

	var resp ItemResult
	_, err := d.requestWithRetry(endpoint, "POST", func(req *resty.Request) {
		req.SetFormData(map[string]string{
			paramName:      extractID(srcObj.GetID()),
			"tofolderid":   extractID(dstDir.GetID()),
			"toname":       srcObj.GetName(),
		})
	}, &resp)

	if err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	return nil
}

// Rename implements driver.Rename
func (d *PCloud) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	endpoint := "/renamefile"
	paramName := "fileid"

	if srcObj.IsDir() {
		endpoint = "/renamefolder"
		paramName = "folderid"
	}

	var resp ItemResult
	_, err := d.requestWithRetry(endpoint, "POST", func(req *resty.Request) {
		req.SetFormData(map[string]string{
			paramName: extractID(srcObj.GetID()),
			"toname":  newName,
		})
	}, &resp)

	if err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	return nil
}

// Copy implements driver.Copy
func (d *PCloud) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	endpoint := "/copyfile"
	paramName := "fileid"

	if srcObj.IsDir() {
		endpoint = "/copyfolder"
		paramName = "folderid"
	}

	var resp ItemResult
	_, err := d.requestWithRetry(endpoint, "POST", func(req *resty.Request) {
		req.SetFormData(map[string]string{
			paramName:    extractID(srcObj.GetID()),
			"tofolderid": extractID(dstDir.GetID()),
			"toname":     srcObj.GetName(),
		})
	}, &resp)

	if err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	return nil
}

// Remove implements driver.Remove
func (d *PCloud) Remove(ctx context.Context, obj model.Obj) error {
	return d.delete(obj.GetID(), obj.IsDir())
}

// Put implements driver.Put
func (d *PCloud) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	parentID := d.RootFolderID
	if dstDir.GetID() != "" {
		parentID = dstDir.GetID()
	}

	return d.uploadFile(ctx, stream, parentID, stream.GetName(), stream.GetSize())
}