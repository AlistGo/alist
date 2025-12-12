package wps

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

const endpoint = "https://365.kdocs.cn"

type resolvedNode struct {
	kind  string
	group Group
	file  *FileInfo
}

func (d *Wps) request(ctx context.Context) *resty.Request {
	return base.RestyClient.R().
		SetHeader("Cookie", d.Cookie).
		SetHeader("Accept", "application/json").
		SetContext(ctx)
}

func (d *Wps) ensureCompanyID(ctx context.Context) error {
	if d.companyID != "" {
		return nil
	}
	var resp workspaceResp
	_, err := d.request(ctx).SetResult(&resp).Get(endpoint + "/3rd/plussvr/compose/v1/users/self/workspaces?fields=name&comp_status=active")
	if err != nil {
		return err
	}
	if len(resp.Companies) == 0 {
		return fmt.Errorf("no company id")
	}
	d.companyID = strconv.FormatInt(resp.Companies[0].ID, 10)
	return nil
}

func (d *Wps) getGroups(ctx context.Context) ([]Group, error) {
	if err := d.ensureCompanyID(ctx); err != nil {
		return nil, err
	}
	var resp groupsResp
	url := fmt.Sprintf("%s/3rd/plus/groups/v1/companies/%s/users/self/groups/private", endpoint, d.companyID)
	_, err := d.request(ctx).SetResult(&resp).Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Groups, nil
}

func (d *Wps) getFiles(ctx context.Context, groupID, parentID int64) ([]FileInfo, error) {
	var resp filesResp
	url := fmt.Sprintf("%s/3rd/drive/api/v5/groups/%d/files", endpoint, groupID)
	_, err := d.request(ctx).SetQueryParam("parentid", strconv.FormatInt(parentID, 10)).SetResult(&resp).Get(url)
	if err != nil {
		return nil, err
	}
	return resp.Files, nil
}

func checkResult(result, msg string) error {
	if result == "" || result == "ok" {
		return nil
	}
	if msg != "" {
		return fmt.Errorf("%s: %s", result, msg)
	}
	return fmt.Errorf("%s", result)
}

func (d *Wps) renameFile(ctx context.Context, groupID, fileID int64, name string) error {
	var resp opResp
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/%d", endpoint, groupID, fileID)
	_, err := d.request(ctx).SetBody(renameReq{Fname: name}).SetResult(&resp).Put(url)
	if err != nil {
		return err
	}
	return checkResult(resp.Result, resp.Msg)
}

func (d *Wps) createFolder(ctx context.Context, groupID, parentID int64, name string) error {
	var resp opResp
	url := endpoint + "/3rd/drive/api/v5/files/folder"
	_, err := d.request(ctx).SetBody(mkdirReq{GroupID: groupID, Name: name, ParentID: parentID}).SetResult(&resp).Post(url)
	if err != nil {
		return err
	}
	return checkResult(resp.Result, resp.Msg)
}

func (d *Wps) moveFile(ctx context.Context, groupID, fileID, targetGroupID, targetParentID int64) error {
	var resp opResp
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/move", endpoint, groupID)
	_, err := d.request(ctx).SetBody(moveReq{FileIDs: []int64{fileID}, TargetGroupID: targetGroupID, TargetParentID: targetParentID}).SetResult(&resp).Post(url)
	if err != nil {
		return err
	}
	return checkResult(resp.Result, resp.Msg)
}

func (d *Wps) deleteFile(ctx context.Context, groupID, fileID int64) error {
	var resp opResp
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/delete", endpoint, groupID)
	_, err := d.request(ctx).SetBody(deleteReq{FileIDs: []int64{fileID}}).SetResult(&resp).Post(url)
	if err != nil {
		return err
	}
	return checkResult(resp.Result, resp.Msg)
}

func (d *Wps) copyFile(ctx context.Context, groupID, fileID, targetGroupID, targetParentID int64) error {
	var resp opResp
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/copy", endpoint, groupID)
	_, err := d.request(ctx).SetBody(copyReq{FileIDs: []int64{fileID}, GroupID: groupID, TargetGroupID: targetGroupID, TargetParentID: targetParentID, DuplicatedNameModel: 1}).SetResult(&resp).Post(url)
	if err != nil {
		return err
	}
	return checkResult(resp.Result, resp.Msg)
}

func parseTime(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}

func joinPath(basePath, name string) string {
	if basePath == "" || basePath == "/" {
		return "/" + name
	}
	return strings.TrimRight(basePath, "/") + "/" + name
}

func (d *Wps) resolvePath(ctx context.Context, path string) (*resolvedNode, error) {
	clean := strings.TrimSpace(path)
	if clean == "" {
		clean = "/"
	}
	clean = strings.Trim(clean, "/")
	if clean == "" {
		return &resolvedNode{kind: "root"}, nil
	}
	segs := strings.Split(clean, "/")
	groups, err := d.getGroups(ctx)
	if err != nil {
		return nil, err
	}
	var grp *Group
	for i := range groups {
		if groups[i].Name == segs[0] {
			grp = &groups[i]
			break
		}
	}
	if grp == nil {
		return nil, fmt.Errorf("group not found")
	}
	if len(segs) == 1 {
		return &resolvedNode{kind: "group", group: *grp}, nil
	}
	parentID := int64(0)
	var last FileInfo
	for i := 1; i < len(segs); i++ {
		files, err := d.getFiles(ctx, grp.GroupID, parentID)
		if err != nil {
			return nil, err
		}
		var found *FileInfo
		for j := range files {
			if files[j].Name == segs[i] {
				found = &files[j]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("path not found")
		}
		last = *found
		parentID = found.ID
	}
	kind := "file"
	if last.Type == "folder" {
		kind = "folder"
	}
	return &resolvedNode{kind: kind, group: *grp, file: &last}, nil
}

func fileToObj(basePath string, f FileInfo) *Obj {
	name := f.Name
	path := joinPath(basePath, name)
	return &Obj{
		id:          path,
		name:        name,
		size:        f.Size,
		ctime:       parseTime(f.Ctime),
		mtime:       parseTime(f.Mtime),
		isDir:       f.Type == "folder",
		path:        path,
		canDownload: f.FilePerms.Download != 0,
	}
}
