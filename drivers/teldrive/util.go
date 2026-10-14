package teldrive

import (
	"bytes"
	"fmt"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// do others that not defined in Driver interface

func (d *Teldrive) request(method string, pathname string, callback base.ReqCallback, resp interface{}) error {
	url := d.Address + pathname
	req := base.RestyClient.R()
	req.SetHeader("Cookie", d.Cookie)
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e ErrResp
	req.SetError(&e)
	_req, err := req.Execute(method, url)
	if err != nil {
		return err
	}

	if _req.IsError() {
		return &e
	}
	return nil
}

func (d *Teldrive) getFile(path, name string, isFolder bool) (model.Obj, error) {
	resp := &ListResp{}
	err := d.request(http.MethodGet, "/api/files", func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"path": path,
			"name": name,
			"type": func() string {
				if isFolder {
					return "folder"
				}
				return "file"
			}(),
			"operation": "find",
		})
	}, resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("file not found: %s/%s", path, name)
	}
	obj := resp.Items[0]
	return &model.Object{
		ID:       obj.ID,
		Name:     obj.Name,
		Size:     obj.Size,
		IsFolder: obj.Type == "folder",
	}, err
}

func (err *ErrResp) Error() string {
	if err == nil {
		return ""
	}

	return fmt.Sprintf("[Teldrive] message:%s Error code:%d", err.Message, err.Code)
}

// create empty file
func (d *Teldrive) touch(name, path string) error {
	uploadBody := base.Json{
		"name": name,
		"type": "file",
		"path": path,
	}
	if err := d.request(http.MethodPost, "/api/files", func(req *resty.Request) {
		req.SetBody(uploadBody)
	}, nil); err != nil {
		return err
	}

	return nil
}

func (d *Teldrive) createFileOnUploadSuccess(name, id, path string, uploadedFileParts []FilePart, totalSize int64) error {
	remoteFileParts, err := d.getFilePart(id)
	if err != nil {
		return err
	}
	// check if the uploaded file parts match the remote file parts
	if len(remoteFileParts) != len(uploadedFileParts) {
		return fmt.Errorf("[Teldrive] file parts count mismatch: expected %d, got %d", len(uploadedFileParts), len(remoteFileParts))
	}
	formatParts := make([]base.Json, 0)
	for _, p := range remoteFileParts {
		formatParts = append(formatParts, base.Json{
			"id":   p.PartId,
			"salt": p.Salt,
		})
	}
	uploadBody := base.Json{
		"name":  name,
		"type":  "file",
		"path":  path,
		"parts": formatParts,
		"size":  totalSize,
	}
	// create file here
	if err := d.request(http.MethodPost, "/api/files", func(req *resty.Request) {
		req.SetBody(uploadBody)
	}, nil); err != nil {
		return err
	}

	return nil
}

func (d *Teldrive) checkFilePartExist(fileId string, partId int) (FilePart, error) {
	var uploadedParts []FilePart
	var filePart FilePart

	if err := d.request(http.MethodGet, "/api/uploads/"+fileId, nil, &uploadedParts); err != nil {
		return filePart, err
	}

	for _, part := range uploadedParts {
		if part.PartId == partId {
			return part, nil
		}
	}

	return filePart, nil
}

func (d *Teldrive) getFilePart(fileId string) ([]FilePart, error) {
	var uploadedParts []FilePart
	if err := d.request(http.MethodGet, "/api/uploads/"+fileId, nil, &uploadedParts); err != nil {
		return nil, err
	}

	return uploadedParts, nil
}

func (d *Teldrive) singleUploadRequest(fileId string, callback base.ReqCallback, resp interface{}) error {
	url := d.Address + "/api/uploads/" + fileId
	client := resty.New().SetTimeout(0)

	ctx := context.Background()

	req := client.R().
		SetContext(ctx)
	req.SetHeader("Cookie", d.Cookie)
	req.SetHeader("Content-Type", "application/octet-stream")
	req.SetContentLength(true)
	req.AddRetryCondition(func(r *resty.Response, err error) bool {
		return false
	})
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e ErrResp
	req.SetError(&e)
	_req, err := req.Execute(http.MethodPost, url)
	if err != nil {
		return err
	}

	if _req.IsError() {
		return &e
	}
	return nil
}

func (d *Teldrive) doSingleUpload(ctx context.Context, dstDir model.Obj, file model.FileStreamer, p *driver.Progress,
	retryCount, maxRetried, totalParts int, fileId string) error {

	chunkIdx := 1
	totalSize := file.GetSize()
	var fileParts []FilePart
	for p.Done < p.Total {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}
		// only one chunk, so we can use the whole file
		byteData := make([]byte, totalSize)
		_, err := io.ReadFull(file, byteData)
		if err != io.EOF && err != nil {
			return err
		}
		filePart := &FilePart{}
		// be sure the file is uploaded, and break loop if success
		for {
			if err := d.singleUploadRequest(fileId, func(req *resty.Request) {
				uploadParams := map[string]string{
					"partName": func() string {
						digits := len(fmt.Sprintf("%d", totalParts))
						return file.GetName() + fmt.Sprintf("%0*d", digits, chunkIdx)
					}(),
					"partNo":   strconv.Itoa(chunkIdx),
					"fileName": file.GetName(),
				}
				req.SetQueryParams(uploadParams)
				req.SetBody(driver.NewLimitedUploadStream(ctx, bytes.NewReader(byteData)))
				req.SetHeader("Content-Length", strconv.Itoa(len(byteData)))
			}, filePart); err != nil {
				if retryCount >= maxRetried {
					utils.Log.Errorf("[Teldrive] upload failed after %d retries: %s", maxRetried, err.Error())
					return err
				}
				if errors.Is(err, context.DeadlineExceeded) {
					continue
				}
				retryCount++
				errorStr := fmt.Sprintf("[Teldrive] upload error: %v, retrying %d times", err, retryCount)
				utils.Log.Errorf(errorStr)
				time.Sleep(time.Duration(retryCount<<1) * time.Second) // Exponential backoff: 2, 4, 8, 16, ...
				continue
			}
			break
		}
		if filePart.Name != "" {
			fileParts = append(fileParts, *filePart)
			retryCount = 0
			_, _ = p.Write(byteData)
			chunkIdx++
		}

	}

	return d.createFileOnUploadSuccess(file.GetName(), fileId, dstDir.GetPath(), fileParts, totalSize)
}

func (d *Teldrive) doMultiUpload(ctx context.Context, dstDir model.Obj, file model.FileStreamer, p *driver.Progress,
	maxRetried, totalParts int, chunkSize int64, fileId string) error {
	concurrent := d.UploadConcurrency
	g, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(int64(concurrent))
	chunkChan := make(chan chunkTask, concurrent*2)
	resultChan := make(chan FilePart, concurrent)
	totalSize := file.GetSize()
	g.Go(func() error {
		defer close(chunkChan)

		chunkIdx := 1
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if p.Done >= p.Total {
				break
			}

			byteData := make([]byte, chunkSize)
			n, err := io.ReadFull(file, byteData)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					if n > 0 {
						// handle the last chuck
						byteData = byteData[:n]
						task := chunkTask{
							data:     byteData,
							chunkIdx: chunkIdx,
							fileName: file.GetName(),
						}
						select {
						case chunkChan <- task:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					break
				}
				return fmt.Errorf("read file error: %w", err)
			}

			if _, err := p.Write(byteData); err != nil {
				return fmt.Errorf("progress update error: %w", err)
			}

			task := chunkTask{
				data:     byteData,
				chunkIdx: chunkIdx,
				fileName: file.GetName(),
			}

			select {
			case chunkChan <- task:
				chunkIdx++
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	for i := 0; i < int(concurrent); i++ {
		g.Go(func() error {
			for task := range chunkChan {
				if err := sem.Acquire(ctx, 1); err != nil {
					return err
				}

				filePart, err := d.uploadSingleChunk(ctx, fileId, task, totalParts, maxRetried)
				sem.Release(1)

				if err != nil {
					return fmt.Errorf("upload chunk %d failed: %w", task.chunkIdx, err)
				}

				select {
				case resultChan <- *filePart:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	var fileParts []FilePart
	var collectErr error
	collectDone := make(chan struct{})

	go func() {
		defer close(collectDone)
		fileParts = make([]FilePart, 0, totalParts)

		done := make(chan error, 1)
		go func() {
			done <- g.Wait()
			close(resultChan)
		}()

		for {
			select {
			case filePart, ok := <-resultChan:
				if !ok {
					collectErr = <-done
					return
				}
				fileParts = append(fileParts, filePart)
			case err := <-done:
				collectErr = err
				return
			}
		}
	}()

	<-collectDone

	if collectErr != nil {
		return fmt.Errorf("multi-upload failed: %w", collectErr)
	}
	sort.Slice(fileParts, func(i, j int) bool {
		return fileParts[i].PartNo < fileParts[j].PartNo
	})

	return d.createFileOnUploadSuccess(file.GetName(), fileId, dstDir.GetPath(), fileParts, totalSize)
}

func (d *Teldrive) uploadSingleChunk(ctx context.Context, fileId string, task chunkTask, totalParts, maxRetried int) (*FilePart, error) {
	filePart := &FilePart{}
	retryCount := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if existingPart, err := d.checkFilePartExist(fileId, task.chunkIdx); err == nil && existingPart.Name != "" {
			return &existingPart, nil
		}

		err := d.singleUploadRequest(fileId, func(req *resty.Request) {
			uploadParams := map[string]string{
				"partName": func() string {
					digits := len(fmt.Sprintf("%d", totalParts))
					return task.fileName + fmt.Sprintf("%0*d", digits, task.chunkIdx)
				}(),
				"partNo":   strconv.Itoa(task.chunkIdx),
				"fileName": task.fileName,
			}
			req.SetQueryParams(uploadParams)
			req.SetBody(driver.NewLimitedUploadStream(ctx, bytes.NewReader(task.data)))
			req.SetHeader("Content-Length", strconv.Itoa(len(task.data)))
		}, filePart)

		if err == nil {
			return filePart, nil
		}

		if retryCount >= maxRetried {
			return nil, fmt.Errorf("upload failed after %d retries: %w", maxRetried, err)
		}

		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			continue
		}

		retryCount++
		utils.Log.Errorf("[Teldrive] upload error: %v, retrying %d times", err, retryCount)

		backoffDuration := time.Duration(retryCount*retryCount) * time.Second
		if backoffDuration > 30*time.Second {
			backoffDuration = 30 * time.Second
		}

		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (d *Teldrive) createShareFile(fileId string) error {
	var errResp ErrResp
	if err := d.request(http.MethodPost, "/api/files/"+fileId+"/share", func(req *resty.Request) {
		req.SetBody(base.Json{
			"expiresAt": getDateTime(),
		})
	}, &errResp); err != nil {
		return err
	}

	if errResp.Message != "" {
		return &errResp
	}

	return nil
}

func (d *Teldrive) getShareFileById(fileId string) (*ShareObj, error) {
	var shareObj ShareObj
	if err := d.request(http.MethodGet, "/api/files/"+fileId+"/share", nil, &shareObj); err != nil {
		return nil, err
	}

	return &shareObj, nil
}

func getDateTime() string {
	now := time.Now().UTC()
	formattedWithMs := now.Add(time.Hour * 1).Format("2006-01-02T15:04:05.000Z")
	return formattedWithMs
}

func NewCopyManager(ctx context.Context, concurrent int, d *Teldrive) *CopyManager {
	g, ctx := errgroup.WithContext(ctx)

	return &CopyManager{
		TaskChan: make(chan CopyTask, concurrent*2),
		Sem:      semaphore.NewWeighted(int64(concurrent)),
		G:        g,
		Ctx:      ctx,
		d:        d,
	}
}

func (cm *CopyManager) startWorkers() {
	workerCount := cap(cm.TaskChan) / 2
	for i := 0; i < workerCount; i++ {
		cm.G.Go(func() error {
			return cm.worker()
		})
	}
}

func (cm *CopyManager) worker() error {
	for {
		select {
		case task, ok := <-cm.TaskChan:
			if !ok {
				return nil
			}

			if err := cm.Sem.Acquire(cm.Ctx, 1); err != nil {
				return err
			}

			var err error

			err = cm.processFile(task)

			cm.Sem.Release(1)

			if err != nil {
				return fmt.Errorf("task processing failed: %w", err)
			}

		case <-cm.Ctx.Done():
			return cm.Ctx.Err()
		}
	}
}

func (cm *CopyManager) generateTasks(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcObj.IsDir() {
		return cm.generateFolderTasks(ctx, srcObj, dstDir)
	} else {
		// add single file task directly
		select {
		case cm.TaskChan <- CopyTask{SrcObj: srcObj, DstDir: dstDir}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (cm *CopyManager) generateFolderTasks(ctx context.Context, srcDir, dstDir model.Obj) error {
	objs, err := cm.d.List(ctx, srcDir, model.ListArgs{})
	if err != nil {
		return fmt.Errorf("failed to list directory %s: %w", srcDir.GetPath(), err)
	}

	err = cm.d.MakeDir(cm.Ctx, dstDir, srcDir.GetName())
	if err != nil || len(objs) == 0 {
		return err
	}
	newDstDir := &model.Object{
		ID:       dstDir.GetID(),
		Path:     dstDir.GetPath() + "/" + srcDir.GetName(),
		Name:     srcDir.GetName(),
		IsFolder: true,
	}

	for _, file := range objs {
		if utils.IsCanceled(ctx) {
			return ctx.Err()
		}

		srcFile := &model.Object{
			ID:       file.GetID(),
			Path:     srcDir.GetPath() + "/" + file.GetName(),
			Name:     file.GetName(),
			IsFolder: file.IsDir(),
		}

		// 递归生成任务
		if err := cm.generateTasks(ctx, srcFile, newDstDir); err != nil {
			return err
		}
	}

	return nil
}

func (cm *CopyManager) processFile(task CopyTask) error {
	return cm.copySingleFile(cm.Ctx, task.SrcObj, task.DstDir)
}

func (cm *CopyManager) copySingleFile(ctx context.Context, srcObj, dstDir model.Obj) error {
	// `override copy mode` should delete the existing file
	if obj, err := cm.d.getFile(dstDir.GetPath(), srcObj.GetName(), srcObj.IsDir()); err == nil {
		if err := cm.d.Remove(ctx, obj); err != nil {
			return fmt.Errorf("failed to remove existing file: %w", err)
		}
	}

	// Do copy
	return cm.d.request(http.MethodPost, "/api/files/"+srcObj.GetID()+"/copy", func(req *resty.Request) {
		req.SetBody(base.Json{
			"newName":     srcObj.GetName(),
			"destination": dstDir.GetPath(),
		})
	}, nil)
}
