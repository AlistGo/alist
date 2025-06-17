package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	stdpath "path"
	"strings"
	"sync"
	"text/template"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Github struct {
	model.Storage
	Addition
	client        *resty.Client
	mkdirMsgTmpl  *template.Template
	deleteMsgTmpl *template.Template
	putMsgTmpl    *template.Template
	renameMsgTmpl *template.Template
	copyMsgTmpl   *template.Template
	moveMsgTmpl   *template.Template
	isOnBranch    bool
	commitMutex   sync.Mutex
	pgpEntity     *openpgp.Entity
}

func (d *Github) Config() driver.Config {
	return config
}

func (d *Github) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Github) Init(ctx context.Context) error {
	d.RootFolderPath = utils.FixAndCleanPath(d.RootFolderPath)
	if d.CommitterName != "" && d.CommitterEmail == "" {
		return errors.New("committer email is required")
	}
	if d.CommitterName == "" && d.CommitterEmail != "" {
		return errors.New("committer name is required")
	}
	if d.AuthorName != "" && d.AuthorEmail == "" {
		return errors.New("author email is required")
	}
	if d.AuthorName == "" && d.AuthorEmail != "" {
		return errors.New("author name is required")
	}
	var err error
	d.mkdirMsgTmpl, err = template.New("mkdirCommitMsgTemplate").Parse(d.MkdirCommitMsg)
	if err != nil {
		return err
	}
	d.deleteMsgTmpl, err = template.New("deleteCommitMsgTemplate").Parse(d.DeleteCommitMsg)
	if err != nil {
		return err
	}
	d.putMsgTmpl, err = template.New("putCommitMsgTemplate").Parse(d.PutCommitMsg)
	if err != nil {
		return err
	}
	d.renameMsgTmpl, err = template.New("renameCommitMsgTemplate").Parse(d.RenameCommitMsg)
	if err != nil {
		return err
	}
	d.copyMsgTmpl, err = template.New("copyCommitMsgTemplate").Parse(d.CopyCommitMsg)
	if err != nil {
		return err
	}
	d.moveMsgTmpl, err = template.New("moveCommitMsgTemplate").Parse(d.MoveCommitMsg)
	if err != nil {
		return err
	}
	d.client = base.NewRestyClient().
		SetHeader("Accept", "application/vnd.github.object+json").
		SetHeader("X-GitHub-Api-Version", "2022-11-28").
		SetLogger(log.StandardLogger()).
		SetDebug(false)
	token := strings.TrimSpace(d.Token)
	if token != "" {
		d.client = d.client.SetHeader("Authorization", "Bearer "+token)
	}
	if d.Ref == "" {
		repo, err := d.getRepo()
		if err != nil {
			return err
		}
		d.Ref = repo.DefaultBranch
		d.isOnBranch = true
	} else {
		_, err = d.getBranchHead()
		d.isOnBranch = err == nil
	}
	if d.GPGPrivateKey != "" {
		if d.CommitterName == "" || d.AuthorName == "" {
			user, e := d.getAuthenticatedUser()
			if e != nil {
				return e
			}
			if d.CommitterName == "" {
				d.CommitterName = user.Name
				d.CommitterEmail = user.Email
			}
			if d.AuthorName == "" {
				d.AuthorName = user.Name
				d.AuthorEmail = user.Email
			}
		}
		d.pgpEntity, err = loadPrivateKey(d.GPGPrivateKey, d.GPGKeyPassphrase)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Github) Drop(ctx context.Context) error {
	return nil
}

func (d *Github) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	obj, err := d.get(dir.GetPath())
	if err != nil {
		return nil, err
	}
	if obj.Entries == nil {
		return nil, errs.NotFolder
	}
	if len(obj.Entries) >= 1000 {
		tree, err := d.getTree(obj.Sha)
		if err != nil {
			return nil, err
		}
		if tree.Truncated {
			return nil, fmt.Errorf("tree %s is truncated", dir.GetPath())
		}
		ret := make([]model.Obj, 0, len(tree.Trees))
		for _, t := range tree.Trees {
			if t.Path != ".gitkeep" {
				ret = append(ret, t.toModelObj())
			}
		}
		return ret, nil
	} else {
		ret := make([]model.Obj, 0, len(obj.Entries))
		for _, entry := range obj.Entries {
			if entry.Name != ".gitkeep" {
				ret = append(ret, entry.toModelObj())
			}
		}
		return ret, nil
	}
}

func (d *Github) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	obj, err := d.get(file.GetPath())
	if err != nil {
		return nil, err
	}
	if obj.Type == "submodule" {
		return nil, errors.New("cannot download a submodule")
	}
	url := obj.DownloadURL
	ghProxy := strings.TrimSpace(d.Addition.GitHubProxy)
	if ghProxy != "" {
		url = strings.Replace(url, "https://raw.githubusercontent.com", ghProxy, 1)
	}
	return &model.Link{
		URL: url,
	}, nil
}

func (d *Github) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()
	parent, err := d.get(parentDir.GetPath())
	if err != nil {
		return err
	}
	if parent.Entries == nil {
		return errs.NotFolder
	}
	subDirSha, err := d.newTree("", []interface{}{
		map[string]string{
			"path":    ".gitkeep",
			"mode":    "100644",
			"type":    "blob",
			"content": "",
		},
	})
	if err != nil {
		return err
	}
	newTree := make([]interface{}, 0, 2)
	newTree = append(newTree, TreeObjReq{
		Path: dirName,
		Mode: "040000",
		Type: "tree",
		Sha:  subDirSha,
	})
	if len(parent.Entries) == 1 && parent.Entries[0].Name == ".gitkeep" {
		newTree = append(newTree, TreeObjReq{
			Path: ".gitkeep",
			Mode: "100644",
			Type: "blob",
			Sha:  nil,
		})
	}
	newSha, err := d.newTree(parent.Sha, newTree)
	if err != nil {
		return err
	}
	rootSha, err := d.renewParentTrees(parentDir.GetPath(), parent.Sha, newSha, "/")
	if err != nil {
		return err
	}

	commitMessage, err := getMessage(d.mkdirMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    dirName,
		ObjPath:    stdpath.Join(parentDir.GetPath(), dirName),
		ParentName: parentDir.GetName(),
		ParentPath: parentDir.GetPath(),
	}, "mkdir")
	if err != nil {
		return err
	}
	return d.commit(commitMessage, rootSha)
}

func (d *Github) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) {
		return errors.New("cannot move parent dir to child")
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()

	var rootSha string
	if strings.HasPrefix(dstDir.GetPath(), stdpath.Dir(srcObj.GetPath())) { // /aa/1 -> /aa/bb/
		dstOldSha, dstNewSha, ancestorOldSha, srcParentTree, err := d.copyWithoutRenewTree(srcObj, dstDir)
		if err != nil {
			return err
		}

		srcParentPath := stdpath.Dir(srcObj.GetPath())
		dstRest := dstDir.GetPath()[len(srcParentPath):]
		if dstRest[0] == '/' {
			dstRest = dstRest[1:]
		}
		dstNextName, _, _ := strings.Cut(dstRest, "/")
		dstNextPath := stdpath.Join(srcParentPath, dstNextName)
		dstNextTreeSha, err := d.renewParentTrees(dstDir.GetPath(), dstOldSha, dstNewSha, dstNextPath)
		if err != nil {
			return err
		}
		var delSrc, dstNextTree *TreeObjReq = nil, nil
		for _, t := range srcParentTree.Trees {
			if t.Path == dstNextName {
				dstNextTree = &t.TreeObjReq
				dstNextTree.Sha = dstNextTreeSha
			}
			if t.Path == srcObj.GetName() {
				delSrc = &t.TreeObjReq
				delSrc.Sha = nil
			}
			if delSrc != nil && dstNextTree != nil {
				break
			}
		}
		if delSrc == nil || dstNextTree == nil {
			return errs.ObjectNotFound
		}
		ancestorNewSha, err := d.newTree(ancestorOldSha, []interface{}{*delSrc, *dstNextTree})
		if err != nil {
			return err
		}
		rootSha, err = d.renewParentTrees(srcParentPath, ancestorOldSha, ancestorNewSha, "/")
		if err != nil {
			return err
		}
	} else if strings.HasPrefix(srcObj.GetPath(), dstDir.GetPath()) { // /aa/bb/1 -> /aa/
		srcParentPath := stdpath.Dir(srcObj.GetPath())
		srcParentTree, srcParentOldSha, err := d.getTreeDirectly(srcParentPath)
		if err != nil {
			return err
		}
		var src *TreeObjReq = nil
		for _, t := range srcParentTree.Trees {
			if t.Path == srcObj.GetName() {
				if t.Type == "commit" {
					return errors.New("cannot move a submodule")
				}
				src = &t.TreeObjReq
				break
			}
		}
		if src == nil {
			return errs.ObjectNotFound
		}

		delSrc := *src
		delSrc.Sha = nil
		delSrcTree := make([]interface{}, 0, 2)
		delSrcTree = append(delSrcTree, delSrc)
		if len(srcParentTree.Trees) == 1 {
			delSrcTree = append(delSrcTree, map[string]string{
				"path":    ".gitkeep",
				"mode":    "100644",
				"type":    "blob",
				"content": "",
			})
		}
		srcParentNewSha, err := d.newTree(srcParentOldSha, delSrcTree)
		if err != nil {
			return err
		}
		srcRest := srcObj.GetPath()[len(dstDir.GetPath()):]
		if srcRest[0] == '/' {
			srcRest = srcRest[1:]
		}
		srcNextName, _, ok := strings.Cut(srcRest, "/")
		if !ok { // /aa/1 -> /aa/
			return errors.New("cannot move in place")
		}
		srcNextPath := stdpath.Join(dstDir.GetPath(), srcNextName)
		srcNextTreeSha, err := d.renewParentTrees(srcParentPath, srcParentOldSha, srcParentNewSha, srcNextPath)
		if err != nil {
			return err
		}

		ancestorTree, ancestorOldSha, err := d.getTreeDirectly(dstDir.GetPath())
		if err != nil {
			return err
		}
		var srcNextTree *TreeObjReq = nil
		for _, t := range ancestorTree.Trees {
			if t.Path == srcNextName {
				srcNextTree = &t.TreeObjReq
				srcNextTree.Sha = srcNextTreeSha
				break
			}
		}
		if srcNextTree == nil {
			return errs.ObjectNotFound
		}
		ancestorNewSha, err := d.newTree(ancestorOldSha, []interface{}{*srcNextTree, *src})
		if err != nil {
			return err
		}
		rootSha, err = d.renewParentTrees(dstDir.GetPath(), ancestorOldSha, ancestorNewSha, "/")
		if err != nil {
			return err
		}
	} else { // /aa/1 -> /bb/
		// do copy
		dstOldSha, dstNewSha, srcParentOldSha, srcParentTree, err := d.copyWithoutRenewTree(srcObj, dstDir)
		if err != nil {
			return err
		}

		// delete src object and create new tree
		var srcNewTree *TreeObjReq = nil
		for _, t := range srcParentTree.Trees {
			if t.Path == srcObj.GetName() {
				srcNewTree = &t.TreeObjReq
				srcNewTree.Sha = nil
				break
			}
		}
		if srcNewTree == nil {
			return errs.ObjectNotFound
		}
		delSrcTree := make([]interface{}, 0, 2)
		delSrcTree = append(delSrcTree, *srcNewTree)
		if len(srcParentTree.Trees) == 1 {
			delSrcTree = append(delSrcTree, map[string]string{
				"path":    ".gitkeep",
				"mode":    "100644",
				"type":    "blob",
				"content": "",
			})
		}
		srcParentNewSha, err := d.newTree(srcParentOldSha, delSrcTree)
		if err != nil {
			return err
		}

		// renew but the common ancestor of srcPath and dstPath
		ancestor, srcChildName, dstChildName, _, _ := getPathCommonAncestor(srcObj.GetPath(), dstDir.GetPath())
		dstNextTreeSha, err := d.renewParentTrees(dstDir.GetPath(), dstOldSha, dstNewSha, stdpath.Join(ancestor, dstChildName))
		if err != nil {
			return err
		}
		srcNextTreeSha, err := d.renewParentTrees(stdpath.Dir(srcObj.GetPath()), srcParentOldSha, srcParentNewSha, stdpath.Join(ancestor, srcChildName))
		if err != nil {
			return err
		}

		// renew the tree of the last common ancestor
		ancestorTree, ancestorOldSha, err := d.getTreeDirectly(ancestor)
		if err != nil {
			return err
		}
		newTree := make([]interface{}, 2)
		srcBind := false
		dstBind := false
		for _, t := range ancestorTree.Trees {
			if t.Path == srcChildName {
				t.Sha = srcNextTreeSha
				newTree[0] = t.TreeObjReq
				srcBind = true
			}
			if t.Path == dstChildName {
				t.Sha = dstNextTreeSha
				newTree[1] = t.TreeObjReq
				dstBind = true
			}
			if srcBind && dstBind {
				break
			}
		}
		if !srcBind || !dstBind {
			return errs.ObjectNotFound
		}
		ancestorNewSha, err := d.newTree(ancestorOldSha, newTree)
		if err != nil {
			return err
		}
		// renew until root
		rootSha, err = d.renewParentTrees(ancestor, ancestorOldSha, ancestorNewSha, "/")
		if err != nil {
			return err
		}
	}

	// commit
	message, err := getMessage(d.moveMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    srcObj.GetName(),
		ObjPath:    srcObj.GetPath(),
		ParentName: stdpath.Base(stdpath.Dir(srcObj.GetPath())),
		ParentPath: stdpath.Dir(srcObj.GetPath()),
		TargetName: stdpath.Base(dstDir.GetPath()),
		TargetPath: dstDir.GetPath(),
	}, "move")
	if err != nil {
		return err
	}
	return d.commit(message, rootSha)
}

func (d *Github) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()
	parentDir := stdpath.Dir(srcObj.GetPath())
	tree, _, err := d.getTreeDirectly(parentDir)
	if err != nil {
		return err
	}
	newTree := make([]interface{}, 2)
	operated := false
	for _, t := range tree.Trees {
		if t.Path == srcObj.GetName() {
			if t.Type == "commit" {
				return errors.New("cannot rename a submodule")
			}
			delCopy := t.TreeObjReq
			delCopy.Sha = nil
			newTree[0] = delCopy
			t.Path = newName
			newTree[1] = t.TreeObjReq
			operated = true
			break
		}
	}
	if !operated {
		return errs.ObjectNotFound
	}
	newSha, err := d.newTree(tree.Sha, newTree)
	if err != nil {
		return err
	}
	rootSha, err := d.renewParentTrees(parentDir, tree.Sha, newSha, "/")
	if err != nil {
		return err
	}
	message, err := getMessage(d.renameMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    srcObj.GetName(),
		ObjPath:    srcObj.GetPath(),
		ParentName: stdpath.Base(parentDir),
		ParentPath: parentDir,
		TargetName: newName,
		TargetPath: stdpath.Join(parentDir, newName),
	}, "rename")
	if err != nil {
		return err
	}
	return d.commit(message, rootSha)
}

func (d *Github) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) {
		return errors.New("cannot copy parent dir to child")
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()

	dstSha, newSha, _, _, err := d.copyWithoutRenewTree(srcObj, dstDir)
	if err != nil {
		return err
	}
	rootSha, err := d.renewParentTrees(dstDir.GetPath(), dstSha, newSha, "/")
	if err != nil {
		return err
	}
	message, err := getMessage(d.copyMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    srcObj.GetName(),
		ObjPath:    srcObj.GetPath(),
		ParentName: stdpath.Base(stdpath.Dir(srcObj.GetPath())),
		ParentPath: stdpath.Dir(srcObj.GetPath()),
		TargetName: stdpath.Base(dstDir.GetPath()),
		TargetPath: dstDir.GetPath(),
	}, "copy")
	if err != nil {
		return err
	}
	return d.commit(message, rootSha)
}

func (d *Github) Remove(ctx context.Context, obj model.Obj) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()
	parentDir := stdpath.Dir(obj.GetPath())
	tree, treeSha, err := d.getTreeDirectly(parentDir)
	if err != nil {
		return err
	}
	var del *TreeObjReq = nil
	for _, t := range tree.Trees {
		if t.Path == obj.GetName() {
			if t.Type == "commit" {
				return errors.New("cannot remove a submodule")
			}
			del = &t.TreeObjReq
			del.Sha = nil
			break
		}
	}
	if del == nil {
		return errs.ObjectNotFound
	}
	newTree := make([]interface{}, 0, 2)
	newTree = append(newTree, *del)
	if len(tree.Trees) == 1 { // completely emptying the repository will get a 404
		newTree = append(newTree, map[string]string{
			"path":    ".gitkeep",
			"mode":    "100644",
			"type":    "blob",
			"content": "",
		})
	}
	newSha, err := d.newTree(treeSha, newTree)
	if err != nil {
		return err
	}
	rootSha, err := d.renewParentTrees(parentDir, treeSha, newSha, "/")
	if err != nil {
		return err
	}
	commitMessage, err := getMessage(d.deleteMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    obj.GetName(),
		ObjPath:    obj.GetPath(),
		ParentName: stdpath.Base(parentDir),
		ParentPath: parentDir,
	}, "remove")
	if err != nil {
		return err
	}
	return d.commit(commitMessage, rootSha)
}

func (d *Github) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	if !d.isOnBranch {
		return errors.New("cannot write to non-branch reference")
	}
	blob, err := d.putBlob(ctx, stream, up)
	if err != nil {
		return err
	}
	d.commitMutex.Lock()
	defer d.commitMutex.Unlock()
	parent, err := d.get(dstDir.GetPath())
	if err != nil {
		return err
	}
	if parent.Entries == nil {
		return errs.NotFolder
	}
	newTree := make([]interface{}, 0, 2)
	newTree = append(newTree, TreeObjReq{
		Path: stream.GetName(),
		Mode: "100644",
		Type: "blob",
		Sha:  blob,
	})
	if len(parent.Entries) == 1 && parent.Entries[0].Name == ".gitkeep" {
		newTree = append(newTree, TreeObjReq{
			Path: ".gitkeep",
			Mode: "100644",
			Type: "blob",
			Sha:  nil,
		})
	}
	newSha, err := d.newTree(parent.Sha, newTree)
	if err != nil {
		return err
	}
	rootSha, err := d.renewParentTrees(dstDir.GetPath(), parent.Sha, newSha, "/")
	if err != nil {
		return err
	}

	commitMessage, err := getMessage(d.putMsgTmpl, &MessageTemplateVars{
		UserName:   getUsername(ctx),
		ObjName:    stream.GetName(),
		ObjPath:    stdpath.Join(dstDir.GetPath(), stream.GetName()),
		ParentName: dstDir.GetName(),
		ParentPath: dstDir.GetPath(),
	}, "upload")
	if err != nil {
		return err
	}
	return d.commit(commitMessage, rootSha)
}

var _ driver.Driver = (*Github)(nil)

func (d *Github) getContentApiUrl(path string) string {
	path = utils.FixAndCleanPath(path)
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/contents%s", d.Owner, d.Repo, path)
}

func (d *Github) get(path string) (*Object, error) {
	res, err := d.client.R().SetQueryParam("ref", d.Ref).Get(d.getContentApiUrl(path))
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != 200 {
		return nil, toErr(res)
	}
	var resp Object
	err = utils.Json.Unmarshal(res.Body(), &resp)
	return &resp, err
}

func (d *Github) putBlob(ctx context.Context, s model.FileStreamer, up driver.UpdateProgress) (string, error) {
	beforeContent := "{\"encoding\":\"base64\",\"content\":\""
	afterContent := "\"}"
	length := int64(len(beforeContent)) + calculateBase64Length(s.GetSize()) + int64(len(afterContent))
	beforeContentReader := strings.NewReader(beforeContent)
	contentReader, contentWriter := io.Pipe()
	go func() {
		encoder := base64.NewEncoder(base64.StdEncoding, contentWriter)
		if _, err := utils.CopyWithBuffer(encoder, s); err != nil {
			_ = contentWriter.CloseWithError(err)
			return
		}
		_ = encoder.Close()
		_ = contentWriter.Close()
	}()
	afterContentReader := strings.NewReader(afterContent)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/git/blobs", d.Owner, d.Repo),
		driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
			Reader: &driver.SimpleReaderWithSize{
				Reader: io.MultiReader(beforeContentReader, contentReader, afterContentReader),
				Size:   length,
			},
			UpdateProgress: up,
		}))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	token := strings.TrimSpace(d.Token)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.ContentLength = length

	res, err := base.HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode != 201 {
		var errMsg ErrResp
		if err = utils.Json.Unmarshal(resBody, &errMsg); err != nil {
			return "", errors.New(res.Status)
		} else {
			return "", fmt.Errorf("%s: %s", res.Status, errMsg.Message)
		}
	}
	var resp PutBlobResp
	if err = utils.Json.Unmarshal(resBody, &resp); err != nil {
		return "", err
	}
	return resp.Sha, nil
}

func (d *Github) renewParentTrees(path, prevSha, curSha, until string) (string, error) {
	for path != until {
		path = stdpath.Dir(path)
		tree, sha, err := d.getTreeDirectly(path)
		if err != nil {
			return "", err
		}
		var newTree *TreeObjReq = nil
		for _, t := range tree.Trees {
			if t.Sha == prevSha {
				newTree = &t.TreeObjReq
				newTree.Sha = curSha
				break
			}
		}
		if newTree == nil {
			return "", errs.ObjectNotFound
		}
		curSha, err = d.newTree(sha, []interface{}{*newTree})
		if err != nil {
			return "", err
		}
		prevSha = sha
	}
	return curSha, nil
}

func (d *Github) getTree(sha string) (*TreeResp, error) {
	res, err := d.client.R().Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s", d.Owner, d.Repo, sha))
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != 200 {
		return nil, toErr(res)
	}
	var resp TreeResp
	if err = utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Github) getTreeDirectly(path string) (*TreeResp, string, error) {
	p, err := d.get(path)
	if err != nil {
		return nil, "", err
	}
	if p.Entries == nil {
		return nil, "", fmt.Errorf("%s is not a folder", path)
	}
	tree, err := d.getTree(p.Sha)
	if err != nil {
		return nil, "", err
	}
	if tree.Truncated {
		return nil, "", fmt.Errorf("tree %s is truncated", path)
	}
	return tree, p.Sha, nil
}

func (d *Github) newTree(baseSha string, tree []interface{}) (string, error) {
	body := &TreeReq{Trees: tree}
	if baseSha != "" {
		body.BaseTree = baseSha
	}
	res, err := d.client.R().SetBody(body).
		Post(fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees", d.Owner, d.Repo))
	if err != nil {
		return "", err
	}
	if res.StatusCode() != 201 {
		return "", toErr(res)
	}
	var resp TreeResp
	if err = utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return "", err
	}
	return resp.Sha, nil
}

func (d *Github) commit(message, treeSha string) error {
	oldCommit, err := d.getBranchHead()
	body := map[string]interface{}{
		"message": message,
		"tree":    treeSha,
		"parents": []string{oldCommit},
	}
	d.addCommitterAndAuthor(&body)
	if d.pgpEntity != nil {
		signature, e := signCommit(&body, d.pgpEntity)
		if e != nil {
			return e
		}
		body["signature"] = signature
	}
	res, err := d.client.R().SetBody(body).Post(fmt.Sprintf("https://api.github.com/repos/%s/%s/git/commits", d.Owner, d.Repo))
	if err != nil {
		return err
	}
	if res.StatusCode() != 201 {
		return toErr(res)
	}
	var resp CommitResp
	if err = utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return err
	}

	// update branch head
	res, err = d.client.R().
		SetBody(&UpdateRefReq{
			Sha:   resp.Sha,
			Force: false,
		}).
		Patch(fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs/heads/%s", d.Owner, d.Repo, d.Ref))
	if err != nil {
		return err
	}
	if res.StatusCode() != 200 {
		return toErr(res)
	}
	return nil
}

func (d *Github) getBranchHead() (string, error) {
	res, err := d.client.R().Get(fmt.Sprintf("https://api.github.com/repos/%s/%s/branches/%s", d.Owner, d.Repo, d.Ref))
	if err != nil {
		return "", err
	}
	if res.StatusCode() != 200 {
		return "", toErr(res)
	}
	var resp BranchResp
	if err = utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return "", err
	}
	return resp.Commit.Sha, nil
}

func (d *Github) copyWithoutRenewTree(srcObj, dstDir model.Obj) (dstSha, newSha, srcParentSha string, srcParentTree *TreeResp, err error) {
	dst, err := d.get(dstDir.GetPath())
	if err != nil {
		return "", "", "", nil, err
	}
	if dst.Entries == nil {
		return "", "", "", nil, errs.NotFolder
	}
	dstSha = dst.Sha
	srcParentPath := stdpath.Dir(srcObj.GetPath())
	srcParentTree, srcParentSha, err = d.getTreeDirectly(srcParentPath)
	if err != nil {
		return "", "", "", nil, err
	}
	var src *TreeObjReq = nil
	for _, t := range srcParentTree.Trees {
		if t.Path == srcObj.GetName() {
			if t.Type == "commit" {
				return "", "", "", nil, errors.New("cannot copy a submodule")
			}
			src = &t.TreeObjReq
			break
		}
	}
	if src == nil {
		return "", "", "", nil, errs.ObjectNotFound
	}

	newTree := make([]interface{}, 0, 2)
	newTree = append(newTree, *src)
	if len(dst.Entries) == 1 && dst.Entries[0].Name == ".gitkeep" {
		newTree = append(newTree, TreeObjReq{
			Path: ".gitkeep",
			Mode: "100644",
			Type: "blob",
			Sha:  nil,
		})
	}
	newSha, err = d.newTree(dstSha, newTree)
	if err != nil {
		return "", "", "", nil, err
	}
	return dstSha, newSha, srcParentSha, srcParentTree, nil
}

func (d *Github) getRepo() (*RepoResp, error) {
	res, err := d.client.R().Get(fmt.Sprintf("https://api.github.com/repos/%s/%s", d.Owner, d.Repo))
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != 200 {
		return nil, toErr(res)
	}
	var resp RepoResp
	if err = utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Github) getAuthenticatedUser() (*UserResp, error) {
	res, err := d.client.R().Get("https://api.github.com/user")
	if err != nil {
		return nil, err
	}
	if res.StatusCode() != 200 {
		return nil, toErr(res)
	}
	resp := &UserResp{}
	if err = utils.Json.Unmarshal(res.Body(), resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (d *Github) addCommitterAndAuthor(m *map[string]interface{}) {
	if d.CommitterName != "" {
		committer := map[string]string{
			"name":  d.CommitterName,
			"email": d.CommitterEmail,
		}
		(*m)["committer"] = committer
	}
	if d.AuthorName != "" {
		author := map[string]string{
			"name":  d.AuthorName,
			"email": d.AuthorEmail,
		}
		(*m)["author"] = author
	}
}
