package url_tree

import (
	"context"
	"errors"
	stdpath "path"
	"strings"
	"sync"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
)

type Urls struct {
	model.Storage
	Addition
	root  *Node
	mutex sync.RWMutex
}

func (d *Urls) Config() driver.Config {
	return config
}

func (d *Urls) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Urls) Init(ctx context.Context) error {
	node, err := BuildTree(d.UrlStructure, d.HeadSize)
	if err != nil {
		return err
	}
	node.calSize()
	d.root = node
	return nil
}

func (d *Urls) Drop(ctx context.Context) error {
	return nil
}

func (d *Urls) Get(ctx context.Context, path string) (model.Obj, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := GetNodeFromRootByPath(d.root, path)
	return nodeToObj(node, path)
}

func (d *Urls) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := GetNodeFromRootByPath(d.root, dir.GetPath())
	log.Debugf("path: %s, node: %+v", dir.GetPath(), node)
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	if node.isFile() {
		return nil, errs.NotFolder
	}
	return utils.SliceConvert(node.Children, func(node *Node) (model.Obj, error) {
		return nodeToObj(node, stdpath.Join(dir.GetPath(), node.Name))
	})
}

func (d *Urls) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	node := GetNodeFromRootByPath(d.root, file.GetPath())
	log.Debugf("path: %s, node: %+v", file.GetPath(), node)
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	if node.isFile() {
		return &model.Link{
			URL: node.Url,
		}, nil
	}
	return nil, errs.NotFile
}

func (d *Urls) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if !d.Writable {
		return nil, errs.PermissionDenied
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	node := GetNodeFromRootByPath(d.root, parentDir.GetPath())
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	if node.isFile() {
		return nil, errs.NotFolder
	}
	dir := &Node{
		Name:  dirName,
		Level: node.Level + 1,
	}
	node.Children = append(node.Children, dir)
	d.updateStorage()
	return nodeToObj(dir, stdpath.Join(parentDir.GetPath(), dirName))
}

func (d *Urls) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if !d.Writable {
		return nil, errs.PermissionDenied
	}
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) {
		return nil, errors.New("cannot move parent dir to child")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	dstNode := GetNodeFromRootByPath(d.root, dstDir.GetPath())
	if dstNode == nil || dstNode.isFile() {
		return nil, errs.NotFolder
	}
	srcDir, srcName := stdpath.Split(srcObj.GetPath())
	srcParentNode := GetNodeFromRootByPath(d.root, srcDir)
	if srcParentNode == nil {
		return nil, errs.ObjectNotFound
	}
	newChildren := make([]*Node, 0, len(srcParentNode.Children))
	var srcNode *Node
	for _, child := range srcParentNode.Children {
		if child.Name == srcName {
			srcNode = child
		} else {
			newChildren = append(newChildren, child)
		}
	}
	if srcNode == nil {
		return nil, errs.ObjectNotFound
	}
	srcParentNode.Children = newChildren
	srcNode.setLevel(dstNode.Level + 1)
	dstNode.Children = append(dstNode.Children, srcNode)
	d.root.calSize()
	d.updateStorage()
	return nodeToObj(srcNode, stdpath.Join(dstDir.GetPath(), srcName))
}

func (d *Urls) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if !d.Writable {
		return nil, errs.PermissionDenied
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	srcNode := GetNodeFromRootByPath(d.root, srcObj.GetPath())
	if srcNode == nil {
		return nil, errs.ObjectNotFound
	}
	srcNode.Name = newName
	d.updateStorage()
	return nodeToObj(srcNode, stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName))
}

func (d *Urls) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if !d.Writable {
		return nil, errs.PermissionDenied
	}
	if strings.HasPrefix(dstDir.GetPath(), srcObj.GetPath()) {
		return nil, errors.New("cannot copy parent dir to child")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	dstNode := GetNodeFromRootByPath(d.root, dstDir.GetPath())
	if dstNode == nil || dstNode.isFile() {
		return nil, errs.NotFolder
	}
	srcNode := GetNodeFromRootByPath(d.root, srcObj.GetPath())
	if srcNode == nil {
		return nil, errs.ObjectNotFound
	}
	newNode := srcNode.deepCopy(dstNode.Level + 1)
	dstNode.Children = append(dstNode.Children, newNode)
	d.root.calSize()
	d.updateStorage()
	return nodeToObj(newNode, stdpath.Join(dstDir.GetPath(), stdpath.Base(srcObj.GetPath())))
}

func (d *Urls) Remove(ctx context.Context, obj model.Obj) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	objDir, objName := stdpath.Split(obj.GetPath())
	nodeParent := GetNodeFromRootByPath(d.root, objDir)
	if nodeParent == nil {
		return errs.ObjectNotFound
	}
	newChildren := make([]*Node, 0, len(nodeParent.Children))
	var deletedObj *Node
	for _, child := range nodeParent.Children {
		if child.Name != objName {
			newChildren = append(newChildren, child)
		} else {
			deletedObj = child
		}
	}
	if deletedObj == nil {
		return errs.ObjectNotFound
	}
	nodeParent.Children = newChildren
	if deletedObj.Size > 0 {
		d.root.calSize()
	}
	d.updateStorage()
	return nil
}

func (d *Urls) PutURL(ctx context.Context, dstDir model.Obj, name, url string) (model.Obj, error) {
	if !d.Writable {
		return nil, errs.PermissionDenied
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	dirNode := GetNodeFromRootByPath(d.root, dstDir.GetPath())
	if dirNode == nil || dirNode.isFile() {
		return nil, errs.NotFolder
	}
	newNode := &Node{
		Name:  name,
		Level: dirNode.Level + 1,
		Url:   url,
	}
	dirNode.Children = append(dirNode.Children, newNode)
	if d.HeadSize {
		size, err := getSizeFromUrl(url)
		if err != nil {
			log.Errorf("get size from url error: %s", err)
		} else {
			newNode.Size = size
			d.root.calSize()
		}
	}
	d.updateStorage()
	return nodeToObj(newNode, stdpath.Join(dstDir.GetPath(), name))
}

func (d *Urls) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	if !d.Writable {
		return errs.PermissionDenied
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	node := GetNodeFromRootByPath(d.root, dstDir.GetPath()) // parent
	if node == nil {
		return errs.ObjectNotFound
	}
	if node.isFile() {
		return errs.NotFolder
	}
	file, err := parseFileLine(stream.GetName(), d.HeadSize)
	if err != nil {
		return err
	}
	node.Children = append(node.Children, file)
	d.updateStorage()
	return nil
}

func (d *Urls) updateStorage() {
	d.UrlStructure = StringifyTree(d.root)
	op.MustSaveDriverStorage(d)
}

//func (d *Template) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Urls)(nil)
