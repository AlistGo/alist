package url_tree_test

import (
	"context"
	"testing"

	"github.com/alist-org/alist/v3/drivers/url_tree"
	"github.com/alist-org/alist/v3/internal/model"
)

func testTree() (*url_tree.Node, error) {
	text := `folder1:
  name1:https://url1
  http://url2
  folder2:
    http://url3
    http://url4
  http://url5
folder3:
  http://url6
  http://url7
http://url8`
	return url_tree.BuildTree(text, false)
}

func TestBuildTree(t *testing.T) {
	node, err := testTree()
	if err != nil {
		t.Errorf("failed to build tree: %+v", err)
	} else {
		t.Logf("tree: %+v", node)
	}
}

func TestGetNode(t *testing.T) {
	root, err := testTree()
	if err != nil {
		t.Errorf("failed to build tree: %+v", err)
		return
	}
	node := url_tree.GetNodeFromRootByPath(root, "/")
	if node != root {
		t.Errorf("got wrong node: %+v", node)
	}
	url3 := url_tree.GetNodeFromRootByPath(root, "/folder1/folder2/url3")
	if url3 != root.Children[0].Children[2].Children[0] {
		t.Errorf("got wrong node: %+v", url3)
	}
}

func TestDownloadParams(t *testing.T) {
	d := &url_tree.Urls{
		Addition: url_tree.Addition{
			UrlStructure:   "测试文件.mp4:https://cdn.example.com/random123",
			DownloadParams: "attname={{.Name}}",
		},
	}
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("failed to init: %+v", err)
	}
	file, err := d.Get(context.Background(), "/测试文件.mp4")
	if err != nil {
		t.Fatalf("failed to get file: %+v", err)
	}

	// preview must keep the original (clean) URL
	previewLink, err := d.Link(context.Background(), file, model.LinkArgs{Type: "preview"})
	if err != nil {
		t.Fatalf("failed to get preview link: %+v", err)
	}
	if previewLink.URL != "https://cdn.example.com/random123" {
		t.Errorf("preview link should be clean, got: %s", previewLink.URL)
	}

	// download must append the encoded custom parameters
	downLink, err := d.Link(context.Background(), file, model.LinkArgs{})
	if err != nil {
		t.Fatalf("failed to get download link: %+v", err)
	}
	want := "https://cdn.example.com/random123?attname=%E6%B5%8B%E8%AF%95%E6%96%87%E4%BB%B6.mp4"
	if downLink.URL != want {
		t.Errorf("download link mismatch:\n got: %s\nwant: %s", downLink.URL, want)
	}
}
