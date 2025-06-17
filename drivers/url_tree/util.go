package url_tree

import (
	"fmt"
	stdpath "path"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	log "github.com/sirupsen/logrus"
)

// build tree from text, text structure definition:
/**
 * FolderName:
 *   [FileName:][FileSize:][Modified:]Url
 */
/**
 * For example:
 * folder1:
 *   name1:url1
 *   url2
 *   folder2:
 *     url3
 *     url4
 *   url5
 * folder3:
 *   url6
 *   url7
 * url8
 */
// if there are no name, use the last segment of url as name
func BuildTree(text string, headSize bool) (*Node, error) {
	lines := strings.Split(text, "\n")
	var root = &Node{Level: -1, Name: "root"}
	stack := []*Node{root}
	for _, line := range lines {
		// calculate indent
		indent := 0
		for i := 0; i < len(line); i++ {
			if line[i] != ' ' {
				break
			}
			indent++
		}
		// if indent is not a multiple of 2, it is an error
		if indent%2 != 0 {
			return nil, fmt.Errorf("the line '%s' is not a multiple of 2", line)
		}
		// calculate level
		level := indent / 2
		line = strings.TrimSpace(line[indent:])
		// if the line is empty, skip
		if line == "" {
			continue
		}
		// if level isn't greater than the level of the top of the stack
		// it is not the child of the top of the stack
		for level <= stack[len(stack)-1].Level {
			// pop the top of the stack
			stack = stack[:len(stack)-1]
		}
		// if the line is a folder
		if isFolder(line) {
			// create a new node
			node := &Node{
				Level: level,
				Name:  strings.TrimSuffix(line, ":"),
			}
			// add the node to the top of the stack
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, node)
			// push the node to the stack
			stack = append(stack, node)
		} else {
			// if the line is a file
			// create a new node
			node, err := parseFileLine(line, headSize)
			if err != nil {
				return nil, err
			}
			node.Level = level
			// add the node to the top of the stack
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, node)
		}
	}
	return root, nil
}

func isFolder(line string) bool {
	return strings.HasSuffix(line, ":")
}

// line definition:
// [FileName:][FileSize:][Modified:]Url
func parseFileLine(line string, headSize bool) (*Node, error) {
	// if there is no url, it is an error
	if !strings.Contains(line, "http://") && !strings.Contains(line, "https://") {
		return nil, fmt.Errorf("invalid line: %s, because url is required for file", line)
	}
	index := strings.Index(line, "http://")
	if index == -1 {
		index = strings.Index(line, "https://")
	}
	url := line[index:]
	info := line[:index]
	node := &Node{
		Url: url,
	}
	haveSize := false
	if index > 0 {
		if !strings.HasSuffix(info, ":") {
			return nil, fmt.Errorf("invalid line: %s, because file info must end with ':'", line)
		}
		info = info[:len(info)-1]
		if info == "" {
			return nil, fmt.Errorf("invalid line: %s, because file name can't be empty", line)
		}
		infoParts := strings.Split(info, ":")
		node.Name = infoParts[0]
		if len(infoParts) > 1 {
			size, err := strconv.ParseInt(infoParts[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid line: %s, because file size must be an integer", line)
			}
			node.Size = size
			haveSize = true
			if len(infoParts) > 2 {
				modified, err := strconv.ParseInt(infoParts[2], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid line: %s, because file modified must be an unix timestamp", line)
				}
				node.Modified = modified
			}
		}
	} else {
		node.Name = stdpath.Base(url)
	}
	if !haveSize && headSize {
		size, err := getSizeFromUrl(url)
		if err != nil {
			log.Errorf("get size from url error: %s", err)
		} else {
			node.Size = size
		}
	}
	return node, nil
}

func splitPath(path string) []string {
	if path == "/" {
		return []string{"root"}
	}
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	parts := strings.Split(path, "/")
	parts[0] = "root"
	return parts
}

func GetNodeFromRootByPath(root *Node, path string) *Node {
	return root.getByPath(splitPath(path))
}

func nodeToObj(node *Node, path string) (model.Obj, error) {
	if node == nil {
		return nil, errs.ObjectNotFound
	}
	return &model.Object{
		Name:     node.Name,
		Size:     node.Size,
		Modified: time.Unix(node.Modified, 0),
		IsFolder: !node.isFile(),
		Path:     path,
	}, nil
}

func getSizeFromUrl(url string) (int64, error) {
	res, err := base.RestyClient.R().SetDoNotParseResponse(true).Head(url)
	if err != nil {
		return 0, err
	}
	defer res.RawResponse.Body.Close()
	if res.StatusCode() >= 300 {
		return 0, fmt.Errorf("get size from url %s failed, status code: %d", url, res.StatusCode())
	}
	size, err := strconv.ParseInt(res.Header().Get("Content-Length"), 10, 64)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func StringifyTree(node *Node) string {
	sb := strings.Builder{}
	if node.Level == -1 {
		for i, child := range node.Children {
			sb.WriteString(StringifyTree(child))
			if i < len(node.Children)-1 {
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}
	for i := 0; i < node.Level; i++ {
		sb.WriteString("  ")
	}
	if node.Url == "" {
		sb.WriteString(node.Name)
		sb.WriteString(":")
		for _, child := range node.Children {
			sb.WriteString("\n")
			sb.WriteString(StringifyTree(child))
		}
	} else if node.Size == 0 && node.Modified == 0 {
		if stdpath.Base(node.Url) == node.Name {
			sb.WriteString(node.Url)
		} else {
			sb.WriteString(fmt.Sprintf("%s:%s", node.Name, node.Url))
		}
	} else {
		sb.WriteString(node.Name)
		sb.WriteString(":")
		if node.Size != 0 || node.Modified != 0 {
			sb.WriteString(strconv.FormatInt(node.Size, 10))
			sb.WriteString(":")
		}
		if node.Modified != 0 {
			sb.WriteString(strconv.FormatInt(node.Modified, 10))
			sb.WriteString(":")
		}
		sb.WriteString(node.Url)
	}
	return sb.String()
}
