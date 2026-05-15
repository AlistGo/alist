package guangyapan

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
)

func (d *GuangYaPan) ResolveOfflineResource(ctx context.Context, fileURL string) (*OfflineResolveData, error) {
	if err := d.ensureAccessToken(ctx); err != nil {
		return nil, err
	}
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return nil, errors.New("offline url is empty")
	}

	var resp offlineResolveResp
	if err := d.postAPI(ctx, "/cloudcollection/v1/resolve_res", map[string]any{
		"url": fileURL,
	}, &resp); err != nil {
		return nil, err
	}
	if !isSuccessMsg(resp.Msg) {
		return nil, fmt.Errorf("resolve offline resource failed: %s", strings.TrimSpace(resp.Msg))
	}
	return &resp.Data, nil
}

func (d *GuangYaPan) OfflineDownload(ctx context.Context, fileURL string, parentDir model.Obj, fileName string) (*OfflineTask, error) {
	resolved, err := d.ResolveOfflineResource(ctx, fileURL)
	if err != nil {
		return nil, err
	}

	parentID := parentDir.GetID()
	rootID, err := d.getRootFolderID(ctx)
	if err != nil {
		return nil, err
	}
	if parentID == rootID {
		parentID = ""
	}

	taskURL := strings.TrimSpace(resolved.URL)
	if taskURL == "" {
		taskURL = strings.TrimSpace(fileURL)
	}
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = resolved.defaultName(taskURL)
	}

	body := map[string]any{
		"url":      taskURL,
		"parentId": parentID,
		"newName":  name,
	}
	if indexes := resolved.fileIndexes(); len(indexes) > 0 {
		body["fileIndexes"] = indexes
	}

	var resp offlineCreateResp
	if err := d.postAPI(ctx, "/cloudcollection/v1/create_task", body, &resp); err != nil {
		return nil, err
	}
	if !isSuccessMsg(resp.Msg) {
		return nil, fmt.Errorf("create offline task failed: %s", strings.TrimSpace(resp.Msg))
	}
	taskID := strings.TrimSpace(resp.Data.TaskID)
	if taskID == "" {
		return nil, errors.New("create offline task failed: empty task id")
	}
	return &OfflineTask{
		TaskID:   taskID,
		FileName: name,
		Res:      taskURL,
	}, nil
}

func (d *GuangYaPan) OfflineList(ctx context.Context, taskIDs []string, statuses []int, cursor string, pageSize int) ([]OfflineTask, error) {
	if err := d.ensureAccessToken(ctx); err != nil {
		return nil, err
	}
	body := map[string]any{}
	if len(taskIDs) > 0 {
		body["taskIds"] = taskIDs
	}
	if len(statuses) > 0 {
		body["status"] = statuses
	}
	if cursor = strings.TrimSpace(cursor); cursor != "" {
		body["cursor"] = cursor
	}
	if pageSize > 0 {
		body["pageSize"] = pageSize
	}

	var resp offlineListResp
	if err := d.postAPI(ctx, "/cloudcollection/v1/list_task", body, &resp); err != nil {
		return nil, err
	}
	if !isSuccessMsg(resp.Msg) {
		return nil, fmt.Errorf("list offline tasks failed: %s", strings.TrimSpace(resp.Msg))
	}
	return resp.Data.List, nil
}

func (d *GuangYaPan) DeleteOfflineTasks(ctx context.Context, taskIDs []string, deleteFiles bool) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}
	if len(taskIDs) == 0 {
		return nil
	}

	var resp offlineDeleteResp
	if err := d.postAPI(ctx, "/cloudcollection/v2/delete_task", map[string]any{
		"taskIds": taskIDs,
	}, &resp); err != nil {
		return err
	}
	if !isSuccessMsg(resp.Msg) {
		return fmt.Errorf("delete offline tasks failed: %s", strings.TrimSpace(resp.Msg))
	}
	return nil
}

func (d OfflineResolveData) defaultName(fileURL string) string {
	if d.BTResInfo != nil && strings.TrimSpace(d.BTResInfo.FileName) != "" {
		return strings.TrimSpace(d.BTResInfo.FileName)
	}
	u, err := url.Parse(fileURL)
	if err == nil {
		name := strings.TrimSpace(stdpath.Base(u.Path))
		if name != "" && name != "." && name != "/" {
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
			return name
		}
	}
	return "offline_download"
}

func (d OfflineResolveData) fileIndexes() []int {
	if d.BTResInfo == nil || len(d.BTResInfo.Subfiles) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(d.BTResInfo.Subfiles))
	for i, file := range d.BTResInfo.Subfiles {
		if file.FileIndex != nil {
			indexes = append(indexes, *file.FileIndex)
			continue
		}
		indexes = append(indexes, i)
	}
	return indexes
}

func isSuccessMsg(msg string) bool {
	msg = strings.TrimSpace(msg)
	return msg == "" || strings.EqualFold(msg, "success")
}
