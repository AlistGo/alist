package gofile

import (
	"context"
	"fmt"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
)

type Gofile struct {
	model.Storage
	Addition

	accountId string
}

func (d *Gofile) Config() driver.Config {
	return config
}

func (d *Gofile) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Gofile) Init(ctx context.Context) error {
	if d.APIToken == "" {
		return fmt.Errorf("API token is required")
	}

	// Get account ID
	accountId, err := d.getAccountId(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}
	d.accountId = accountId

	// Get account info to set root folder if not specified
	if d.RootFolderID == "" {
		accountInfo, err := d.getAccountInfo(ctx, accountId)
		if err != nil {
			return fmt.Errorf("failed to get account info: %w", err)
		}
		d.RootFolderID = accountInfo.Data.RootFolder
	}

	// Save driver storage
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *Gofile) Drop(ctx context.Context) error {
	return nil
}

func (d *Gofile) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var folderId string
	if dir.GetID() == "" {
		folderId = d.GetRootId()
	} else {
		folderId = dir.GetID()
	}

	endpoint := fmt.Sprintf("/contents/%s", folderId)

	var response ContentsResponse
	err := d.getJSON(ctx, endpoint, &response)
	if err != nil {
		return nil, err
	}

	var objects []model.Obj

	// Process children or contents
	contents := response.Data.Children
	if contents == nil {
		contents = response.Data.Contents
	}

	for _, content := range contents {
		objects = append(objects, d.convertContentToObj(content))
	}

	return objects, nil
}

func (d *Gofile) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.NotFile
	}

	// Create a direct link for the file
	directLink, err := d.createDirectLink(ctx, file.GetID())
	if err != nil {
		return nil, fmt.Errorf("failed to create direct link: %w", err)
	}

	// Configure cache expiration based on user setting
	link := &model.Link{
		URL: directLink,
	}

	// Only set expiration if LinkExpiry > 0 (0 means no caching)
	if d.LinkExpiry > 0 {
		expiration := time.Duration(d.LinkExpiry) * 24 * time.Hour
		link.Expiration = &expiration
	}

	return link, nil
}

func (d *Gofile) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	var parentId string
	if parentDir.GetID() == "" {
		parentId = d.GetRootId()
	} else {
		parentId = parentDir.GetID()
	}

	data := map[string]interface{}{
		"parentFolderId": parentId,
		"folderName":     dirName,
	}

	var response CreateFolderResponse
	err := d.postJSON(ctx, "/contents/createFolder", data, &response)
	if err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       response.Data.ID,
		Name:     response.Data.Name,
		IsFolder: true,
	}, nil
}

func (d *Gofile) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var dstId string
	if dstDir.GetID() == "" {
		dstId = d.GetRootId()
	} else {
		dstId = dstDir.GetID()
	}

	data := map[string]interface{}{
		"contentsId": srcObj.GetID(),
		"folderId":   dstId,
	}

	err := d.putJSON(ctx, "/contents/move", data, nil)
	if err != nil {
		return nil, err
	}

	// Return updated object
	return &model.Object{
		ID:       srcObj.GetID(),
		Name:     srcObj.GetName(),
		Size:     srcObj.GetSize(),
		Modified: srcObj.ModTime(),
		IsFolder: srcObj.IsDir(),
	}, nil
}

func (d *Gofile) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	data := map[string]interface{}{
		"attribute":      "name",
		"attributeValue": newName,
	}

	var response UpdateResponse
	err := d.putJSON(ctx, fmt.Sprintf("/contents/%s/update", srcObj.GetID()), data, &response)
	if err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       srcObj.GetID(),
		Name:     newName,
		Size:     srcObj.GetSize(),
		Modified: srcObj.ModTime(),
		IsFolder: srcObj.IsDir(),
	}, nil
}

func (d *Gofile) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	var dstId string
	if dstDir.GetID() == "" {
		dstId = d.GetRootId()
	} else {
		dstId = dstDir.GetID()
	}

	data := map[string]interface{}{
		"contentsId": srcObj.GetID(),
		"folderId":   dstId,
	}

	var response CopyResponse
	err := d.postJSON(ctx, "/contents/copy", data, &response)
	if err != nil {
		return nil, err
	}

	// Get the new ID from the response
	newId := srcObj.GetID()
	if response.Data.CopiedContents != nil {
		if id, ok := response.Data.CopiedContents[srcObj.GetID()]; ok {
			newId = id
		}
	}

	return &model.Object{
		ID:       newId,
		Name:     srcObj.GetName(),
		Size:     srcObj.GetSize(),
		Modified: srcObj.ModTime(),
		IsFolder: srcObj.IsDir(),
	}, nil
}

func (d *Gofile) Remove(ctx context.Context, obj model.Obj) error {
	data := map[string]interface{}{
		"contentsId": obj.GetID(),
	}

	return d.deleteJSON(ctx, "/contents", data)
}

func (d *Gofile) Put(ctx context.Context, dstDir model.Obj, fileStreamer model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	var folderId string
	if dstDir.GetID() == "" {
		folderId = d.GetRootId()
	} else {
		folderId = dstDir.GetID()
	}

	response, err := d.uploadFile(ctx, folderId, fileStreamer, up)
	if err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       response.Data.FileId,
		Name:     response.Data.FileName,
		Size:     fileStreamer.GetSize(),
		IsFolder: false,
	}, nil
}

func (d *Gofile) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	return nil, errs.NotImplement
}

func (d *Gofile) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *Gofile) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}

func (d *Gofile) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

var _ driver.Driver = (*Gofile)(nil)
