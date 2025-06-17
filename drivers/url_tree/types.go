package url_tree

import "github.com/alist-org/alist/v3/pkg/utils"

// Node is a node in the folder tree
type Node struct {
	Url      string
	Name     string
	Level    int
	Modified int64
	Size     int64
	Children []*Node
}

func (node *Node) getByPath(paths []string) *Node {
	if len(paths) == 0 || node == nil {
		return nil
	}
	if node.Name != paths[0] {
		return nil
	}
	if len(paths) == 1 {
		return node
	}
	for _, child := range node.Children {
		tmp := child.getByPath(paths[1:])
		if tmp != nil {
			return tmp
		}
	}
	return nil
}

func (node *Node) isFile() bool {
	return node.Url != ""
}

func (node *Node) calSize() int64 {
	if node.isFile() {
		return node.Size
	}
	var size int64 = 0
	for _, child := range node.Children {
		size += child.calSize()
	}
	node.Size = size
	return size
}

func (node *Node) setLevel(level int) {
	node.Level = level
	for _, child := range node.Children {
		child.setLevel(level + 1)
	}
}

func (node *Node) deepCopy(level int) *Node {
	ret := *node
	ret.Level = level
	ret.Children, _ = utils.SliceConvert(ret.Children, func(child *Node) (*Node, error) {
		return child.deepCopy(level + 1), nil
	})
	return &ret
}
