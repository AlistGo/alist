package streamtape

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	log "github.com/sirupsen/logrus"
)

type Streamtape struct {
	model.Storage
	Addition
}

var waitMoreSecondsRe = regexp.MustCompile(`wait\s+(\d+)\s+more\s+seconds?`)

func (d *Streamtape) Config() driver.Config {
	return config
}

func (d *Streamtape) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Streamtape) Init(ctx context.Context) error {
	if strings.TrimSpace(d.APILogin) == "" || strings.TrimSpace(d.APIKey) == "" {
		return errors.New("api_login and api_key are required")
	}
	if d.RootFolderID == "" {
		d.RootFolderID = "0"
	}

	var account accountInfo
	if err := d.callAPI(ctx, "/account/info", nil, &account); err != nil {
		return err
	}

	op.MustSaveDriverStorage(d)
	return nil
}

func (d *Streamtape) Drop(ctx context.Context) error {
	return nil
}

func (d *Streamtape) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	folderID := d.RootFolderID
	if dir.GetID() != "" {
		folderID = folderIDFromObjID(dir.GetID())
	}

	params := map[string]string{}
	if folderID != "" && folderID != "0" {
		params["folder"] = folderID
	}

	var result listFolderResult
	if err := d.callAPI(ctx, "/file/listfolder", params, &result); err != nil {
		return nil, err
	}

	objects := make([]model.Obj, 0, len(result.Folders)+len(result.Files))
	for _, f := range result.Folders {
		objects = append(objects, &model.Object{
			ID:       encodeFolderID(f.ID),
			Name:     f.Name,
			IsFolder: true,
		})
	}
	for _, f := range result.Files {
		objects = append(objects, buildFileObj(f))
	}
	return objects, nil
}

func (d *Streamtape) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.NotFile
	}
	fileID := fileIDFromObjID(file.GetID())
	if fileID == "" {
		return nil, errors.New("empty file id")
	}

	var ticket dlTicketResult
	if err := d.callAPI(ctx, "/file/dlticket", map[string]string{"file": fileID}, &ticket); err != nil {
		return nil, err
	}

	var dl dlResult
	waitSeconds := ticket.WaitTime
	if waitSeconds > 0 {
		timer := time.NewTimer(time.Duration(waitSeconds+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	var err error
	for i := 0; i < 3; i++ {
		err = d.callAPI(ctx, "/file/dl", map[string]string{
			"file":   fileID,
			"ticket": ticket.Ticket,
		}, &dl)
		if err == nil {
			break
		}
		waitSeconds = extractWaitSecondsFromErr(err)
		if waitSeconds <= 0 {
			return nil, err
		}
		timer := time.NewTimer(time.Duration(waitSeconds+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		return nil, err
	}

	finalURL := ensureStreamQuery(dl.URL)
	log.Infof("streamtape direct link file=%s url=%s", fileID, finalURL)
	link := &model.Link{
		URL: finalURL,
		Header: http.Header{
			"Referer": []string{"https://streamtape.com/"},
			"Origin":  []string{"https://streamtape.com"},
		},
	}
	d.applyRangeStrategy(link, file.GetSize())
	return link, nil
}

func extractWaitSecondsFromErr(err error) int {
	if err == nil {
		return 0
	}
	matches := waitMoreSecondsRe.FindStringSubmatch(strings.ToLower(err.Error()))
	if len(matches) < 2 {
		return 0
	}
	seconds, convErr := strconv.Atoi(matches[1])
	if convErr != nil || seconds < 0 {
		return 0
	}
	return seconds
}

func ensureStreamQuery(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("stream") == "" {
		q.Set("stream", "1")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (d *Streamtape) applyRangeStrategy(link *model.Link, size int64) {
	if !d.EnableRangeControl || size <= 0 {
		return
	}

	mode := strings.ToLower(strings.TrimSpace(d.RangeMode))
	if mode == "" {
		mode = "chunk"
	}

	switch mode {
	case "full":
		// Keep single full-tail behavior while still using ranged requests.
		link.Concurrency = 1
		link.PartSize = int(size)
	case "percent":
		percent := d.RangePercent
		if percent <= 0 {
			percent = 15
		}
		if percent > 100 {
			percent = 100
		}
		partSize := size * int64(percent) / 100
		if partSize < 1*1024*1024 {
			partSize = 1 * 1024 * 1024
		}
		if partSize > size {
			partSize = size
		}
		link.Concurrency = 1
		link.PartSize = int(partSize)
	default:
		chunkMB := d.RangeChunkMB
		if chunkMB <= 0 {
			chunkMB = 8
		}
		partSize := int64(chunkMB) * 1024 * 1024
		if partSize > size {
			partSize = size
		}
		concurrency := d.RangeConcurrency
		if concurrency <= 0 {
			concurrency = 4
		}
		link.Concurrency = concurrency
		link.PartSize = int(partSize)
	}
}

func (d *Streamtape) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	pid := d.RootFolderID
	if parentDir.GetID() != "" {
		pid = folderIDFromObjID(parentDir.GetID())
	}

	params := map[string]string{"name": dirName}
	if pid != "" && pid != "0" {
		params["pid"] = pid
	}

	var result createFolderResult
	if err := d.callAPI(ctx, "/file/createfolder", params, &result); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       encodeFolderID(result.FolderID),
		Name:     dirName,
		IsFolder: true,
	}, nil
}

func (d *Streamtape) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if srcObj.IsDir() {
		return nil, errs.NotImplement
	}
	fileID := fileIDFromObjID(srcObj.GetID())
	if fileID == "" {
		return nil, errors.New("empty file id")
	}
	folderID := d.RootFolderID
	if dstDir.GetID() != "" {
		folderID = folderIDFromObjID(dstDir.GetID())
	}
	if folderID == "" || folderID == "0" {
		return nil, fmt.Errorf("streamtape move to root is not supported by API")
	}

	if err := d.callAPI(ctx, "/file/move", map[string]string{
		"file":   fileID,
		"folder": folderID,
	}, nil); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       srcObj.GetID(),
		Name:     srcObj.GetName(),
		Size:     srcObj.GetSize(),
		Modified: srcObj.ModTime(),
		IsFolder: false,
	}, nil
}

func (d *Streamtape) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	endpoint := "/file/rename"
	params := map[string]string{"name": newName}
	if srcObj.IsDir() {
		endpoint = "/file/renamefolder"
		params["folder"] = folderIDFromObjID(srcObj.GetID())
	} else {
		params["file"] = fileIDFromObjID(srcObj.GetID())
	}

	if err := d.callAPI(ctx, endpoint, params, nil); err != nil {
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

func (d *Streamtape) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *Streamtape) Remove(ctx context.Context, obj model.Obj) error {
	endpoint := "/file/delete"
	params := map[string]string{}
	if obj.IsDir() {
		endpoint = "/file/deletefolder"
		params["folder"] = folderIDFromObjID(obj.GetID())
	} else {
		params["file"] = fileIDFromObjID(obj.GetID())
	}
	return d.callAPI(ctx, endpoint, params, nil)
}

func (d *Streamtape) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	folderID := d.RootFolderID
	if dstDir.GetID() != "" {
		folderID = folderIDFromObjID(dstDir.GetID())
	}

	params := map[string]string{}
	if folderID != "" && folderID != "0" {
		params["folder"] = folderID
	}
	if d.Sha256 != "" {
		params["sha256"] = d.Sha256
	}

	var uploadURL uploadURLResult
	if err := d.callAPI(ctx, "/file/ul", params, &uploadURL); err != nil {
		return nil, err
	}

	reader := driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
		Reader:         file,
		UpdateProgress: up,
	})

	res, err := base.RestyClient.R().
		SetContext(ctx).
		SetFileReader("file1", file.GetName(), reader).
		Post(uploadURL.URL)
	if err != nil {
		return nil, err
	}
	if res.StatusCode() >= http.StatusBadRequest {
		return nil, fmt.Errorf("streamtape upload failed: http %d", res.StatusCode())
	}

	uploadedID := extractFileIDFromUploadBody(res.Body())
	if uploadedID == "" {
		list, listErr := d.List(ctx, &model.Object{ID: encodeFolderID(folderID), IsFolder: true}, model.ListArgs{})
		if listErr == nil {
			for _, obj := range list {
				if obj.IsDir() {
					continue
				}
				if obj.GetName() == file.GetName() && (file.GetSize() <= 0 || obj.GetSize() == file.GetSize()) {
					return obj, nil
				}
			}
		}
		return &model.Object{
			Name:     file.GetName(),
			Size:     file.GetSize(),
			IsFolder: false,
		}, nil
	}

	return &model.Object{
		ID:       encodeFileID(uploadedID),
		Name:     file.GetName(),
		Size:     file.GetSize(),
		IsFolder: false,
	}, nil
}

// PutURL initiates a remote upload from an external URL
func (d *Streamtape) PutURL(ctx context.Context, dstDir model.Obj, name, url string) (model.Obj, error) {
	folderID := d.RootFolderID
	if dstDir.GetID() != "" {
		folderID = folderIDFromObjID(dstDir.GetID())
	}

	params := map[string]string{
		"url": url,
	}
	if folderID != "" && folderID != "0" {
		params["folder"] = folderID
	}
	if name != "" {
		params["name"] = name
	}

	var result remoteDlAddResult
	if err := d.callAPI(ctx, "/remotedl/add", params, &result); err != nil {
		return nil, err
	}

	return &model.Object{
		ID:       encodeRemoteUploadID(result.ID),
		Name:     name,
		IsFolder: false,
	}, nil
}

func (d *Streamtape) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	return nil, errs.NotImplement
}

func (d *Streamtape) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *Streamtape) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}

func (d *Streamtape) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	return nil, errs.NotImplement
}

func (d *Streamtape) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	switch strings.ToLower(args.Method) {
	case "remotedl_status":
		return d.remoteDlStatus(ctx, args)
	case "remotedl_remove":
		return d.remoteDlRemove(ctx, args)
	case "file_info":
		return d.fileInfo(ctx, args)
	case "thumbnail":
		return d.thumbnail(ctx, args)
	case "conversion_status":
		return d.conversionStatus(ctx, args)
	default:
		return nil, errs.NotSupport
	}
}

func (d *Streamtape) extractRemoteUploadID(args model.OtherArgs) (string, error) {
	uploadID := remoteUploadIDFromObjID(args.Obj.GetID())
	if uploadID == "" {
		if data, ok := args.Data.(map[string]interface{}); ok {
			if id, ok := data["id"].(string); ok {
				uploadID = id
			}
		}
	}
	if uploadID == "" {
		return "", fmt.Errorf("remote upload ID required")
	}
	return uploadID, nil
}

func (d *Streamtape) remoteDlStatus(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	uploadID, err := d.extractRemoteUploadID(args)
	if err != nil {
		return nil, err
	}

	var result remoteDlStatusResult
	if err := d.callAPI(ctx, "/remotedl/status", map[string]string{"id": uploadID}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Streamtape) remoteDlRemove(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	uploadID, err := d.extractRemoteUploadID(args)
	if err != nil {
		return nil, err
	}

	if err := d.callAPI(ctx, "/remotedl/remove", map[string]string{"id": uploadID}, nil); err != nil {
		return nil, err
	}
	return true, nil
}

func (d *Streamtape) fileInfo(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	var fileIDs string
	if data, ok := args.Data.(map[string]interface{}); ok {
		if ids, ok := data["file_ids"].(string); ok {
			fileIDs = ids
		}
	}
	if fileIDs == "" {
		fileIDs = fileIDFromObjID(args.Obj.GetID())
	}
	if fileIDs == "" {
		return nil, fmt.Errorf("file IDs required")
	}

	var result fileInfoResult
	if err := d.callAPI(ctx, "/file/info", map[string]string{"file": fileIDs}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Streamtape) thumbnail(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	fileID := fileIDFromObjID(args.Obj.GetID())
	if fileID == "" {
		return nil, fmt.Errorf("file ID required")
	}

	var result string
	if err := d.callAPI(ctx, "/file/getsplash", map[string]string{"file": fileID}, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *Streamtape) conversionStatus(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	isFailed := false
	if data, ok := args.Data.(map[string]interface{}); ok {
		if t, ok := data["type"].(string); ok && t == "failed" {
			isFailed = true
		}
	}

	endpoint := "/file/runningconverts"
	if isFailed {
		endpoint = "/file/failedconverts"
	}

	var result conversionResult
	if err := d.callAPI(ctx, endpoint, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

var _ driver.Driver = (*Streamtape)(nil)
