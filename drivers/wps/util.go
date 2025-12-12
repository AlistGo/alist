package wps

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
)

const endpoint = "https://365.kdocs.cn"

type resolvedNode struct {
	kind  string
	group Group
	file  *FileInfo
}

type apiResult struct {
	Result string `json:"result"`
	Msg    string `json:"msg"`
}

func (d *Wps) request(ctx context.Context) *resty.Request {
	return base.RestyClient.R().
		SetHeader("Cookie", d.Cookie).
		SetHeader("Accept", "application/json").
		SetContext(ctx)
}

func (d *Wps) jsonRequest(ctx context.Context) *resty.Request {
	return d.request(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Origin", "https://365.kdocs.cn/")
}

func (d *Wps) ensureCompanyID(ctx context.Context) error {
	if d.companyID != "" {
		return nil
	}
	var resp workspaceResp
	r, err := d.request(ctx).SetResult(&resp).SetError(&resp).Get(endpoint + "/3rd/plussvr/compose/v1/users/self/workspaces?fields=name&comp_status=active")
	if err != nil {
		return err
	}
	if r != nil && r.IsError() {
		return fmt.Errorf("http error: %d", r.StatusCode())
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
	r, err := d.request(ctx).SetResult(&resp).SetError(&resp).Get(url)
	if err != nil {
		return nil, err
	}
	if r != nil && r.IsError() {
		return nil, fmt.Errorf("http error: %d", r.StatusCode())
	}
	return resp.Groups, nil
}

func (d *Wps) getFiles(ctx context.Context, groupID, parentID int64) ([]FileInfo, error) {
	var resp filesResp
	url := fmt.Sprintf("%s/3rd/drive/api/v5/groups/%d/files", endpoint, groupID)
	r, err := d.request(ctx).
		SetQueryParam("parentid", strconv.FormatInt(parentID, 10)).
		SetResult(&resp).
		SetError(&resp).
		Get(url)
	if err != nil {
		return nil, err
	}
	if r != nil && r.IsError() {
		return nil, fmt.Errorf("http error: %d", r.StatusCode())
	}
	return resp.Files, nil
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
		if i < len(segs)-1 && found.Type != "folder" {
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

func (d *Wps) doJSON(ctx context.Context, method, url string, body interface{}) error {
	var result apiResult
	req := d.jsonRequest(ctx).SetBody(body).SetResult(&result).SetError(&result)
	var (
		resp *resty.Response
		err  error
	)
	switch method {
	case http.MethodPost:
		resp, err = req.Post(url)
	case http.MethodPut:
		resp, err = req.Put(url)
	default:
		return errs.NotSupport
	}
	if err != nil {
		return err
	}
	if result.Result != "" && result.Result != "ok" {
		if result.Msg == "" {
			result.Msg = "unknown error"
		}
		return fmt.Errorf("%s: %s", result.Result, result.Msg)
	}
	if resp != nil && resp.IsError() {
		if result.Msg != "" {
			return fmt.Errorf("%s", result.Msg)
		}
		return fmt.Errorf("http error: %d", resp.StatusCode())
	}
	return nil
}

func (d *Wps) list(ctx context.Context, basePath string) ([]model.Obj, error) {
	if strings.TrimSpace(basePath) == "" {
		basePath = "/"
	}
	node, err := d.resolvePath(ctx, basePath)
	if err != nil {
		return nil, err
	}
	if node.kind == "root" {
		groups, err := d.getGroups(ctx)
		if err != nil {
			return nil, err
		}
		res := make([]model.Obj, 0, len(groups))
		for _, g := range groups {
			path := joinPath(basePath, g.Name)
			obj := &Obj{
				id:    path,
				name:  g.Name,
				ctime: parseTime(0),
				mtime: parseTime(0),
				isDir: true,
				path:  path,
			}
			res = append(res, obj)
		}
		return res, nil
	}
	if node.kind != "group" && node.kind != "folder" {
		return nil, nil
	}
	parentID := int64(0)
	if node.file != nil && node.kind == "folder" {
		parentID = node.file.ID
	}
	files, err := d.getFiles(ctx, node.group.GroupID, parentID)
	if err != nil {
		return nil, err
	}
	res := make([]model.Obj, 0, len(files))
	for _, f := range files {
		res = append(res, fileToObj(basePath, f))
	}
	return res, nil
}

func (d *Wps) link(ctx context.Context, path string) (*model.Link, error) {
	node, err := d.resolvePath(ctx, path)
	if err != nil {
		return nil, err
	}
	if node.kind != "file" || node.file == nil {
		return nil, errs.NotSupport
	}
	if node.file.FilePerms.Download == 0 {
		return nil, fmt.Errorf("no download permission")
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v5/groups/%d/files/%d/download?support_checksums=sha1", endpoint, node.group.GroupID, node.file.ID)
	var resp downloadResp
	r, err := d.request(ctx).SetResult(&resp).SetError(&resp).Get(url)
	if err != nil {
		return nil, err
	}
	if r != nil && r.IsError() {
		return nil, fmt.Errorf("http error: %d", r.StatusCode())
	}
	if resp.URL == "" {
		return nil, fmt.Errorf("empty download url")
	}
	return &model.Link{URL: resp.URL, Header: http.Header{}}, nil
}

func (d *Wps) makeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if parentDir == nil {
		return errs.NotSupport
	}
	node, err := d.resolvePath(ctx, parentDir.GetPath())
	if err != nil {
		return err
	}
	if node.kind != "group" && node.kind != "folder" {
		return errs.NotSupport
	}
	parentID := int64(0)
	if node.file != nil && node.kind == "folder" {
		parentID = node.file.ID
	}
	body := map[string]interface{}{
		"groupid":  node.group.GroupID,
		"name":     dirName,
		"parentid": parentID,
	}
	return d.doJSON(ctx, http.MethodPost, endpoint+"/3rd/drive/api/v5/files/folder", body)
}

func (d *Wps) move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcObj == nil || dstDir == nil {
		return errs.NotSupport
	}
	nodeSrc, err := d.resolvePath(ctx, srcObj.GetPath())
	if err != nil {
		return err
	}
	nodeDst, err := d.resolvePath(ctx, dstDir.GetPath())
	if err != nil {
		return err
	}
	if nodeSrc.kind != "file" && nodeSrc.kind != "folder" {
		return errs.NotSupport
	}
	if nodeDst.kind != "group" && nodeDst.kind != "folder" {
		return errs.NotSupport
	}
	targetParentID := int64(0)
	if nodeDst.file != nil && nodeDst.kind == "folder" {
		targetParentID = nodeDst.file.ID
	}
	body := map[string]interface{}{
		"fileids":         []int64{nodeSrc.file.ID},
		"target_groupid":  nodeDst.group.GroupID,
		"target_parentid": targetParentID,
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/move", endpoint, nodeSrc.group.GroupID)
	return d.doJSON(ctx, http.MethodPost, url, body)
}

func (d *Wps) rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if srcObj == nil {
		return errs.NotSupport
	}
	node, err := d.resolvePath(ctx, srcObj.GetPath())
	if err != nil {
		return err
	}
	if node.kind != "file" && node.kind != "folder" {
		return errs.NotSupport
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/%d", endpoint, node.group.GroupID, node.file.ID)
	body := map[string]string{"fname": newName}
	return d.doJSON(ctx, http.MethodPut, url, body)
}

func (d *Wps) copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcObj == nil || dstDir == nil {
		return errs.NotSupport
	}
	nodeSrc, err := d.resolvePath(ctx, srcObj.GetPath())
	if err != nil {
		return err
	}
	nodeDst, err := d.resolvePath(ctx, dstDir.GetPath())
	if err != nil {
		return err
	}
	if nodeSrc.kind != "file" && nodeSrc.kind != "folder" {
		return errs.NotSupport
	}
	if nodeDst.kind != "group" && nodeDst.kind != "folder" {
		return errs.NotSupport
	}
	targetParentID := int64(0)
	if nodeDst.file != nil && nodeDst.kind == "folder" {
		targetParentID = nodeDst.file.ID
	}
	body := map[string]interface{}{
		"fileids":               []int64{nodeSrc.file.ID},
		"groupid":               nodeSrc.group.GroupID,
		"target_groupid":        nodeDst.group.GroupID,
		"target_parentid":       targetParentID,
		"duplicated_name_model": 1,
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/copy", endpoint, nodeSrc.group.GroupID)
	return d.doJSON(ctx, http.MethodPost, url, body)
}

func (d *Wps) remove(ctx context.Context, obj model.Obj) error {
	if obj == nil {
		return errs.NotSupport
	}
	node, err := d.resolvePath(ctx, obj.GetPath())
	if err != nil {
		return err
	}
	if node.kind != "file" && node.kind != "folder" {
		return errs.NotSupport
	}
	body := map[string]interface{}{
		"fileids": []int64{node.file.ID},
	}
	url := fmt.Sprintf("%s/3rd/drive/api/v3/groups/%d/files/batch/delete", endpoint, node.group.GroupID)
	return d.doJSON(ctx, http.MethodPost, url, body)
}
