//go:build windows

package local

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
)

func setHiddenAttr(t *testing.T, path string) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.SetFileAttributes(p, attrs|syscall.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatal(err)
	}
}

func TestListHonorsWindowsHiddenAttribute(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".dotfile", "hidden.txt", "visible.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"$RECYCLE.BIN", "folder"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	setHiddenAttr(t, filepath.Join(dir, "hidden.txt"))
	setHiddenAttr(t, filepath.Join(dir, "$RECYCLE.BIN"))

	list := func(showHidden bool) []string {
		d := &Local{Addition: Addition{ShowHidden: showHidden}}
		objs, err := d.List(context.Background(), &model.Object{Path: dir, IsFolder: true}, model.ListArgs{})
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(objs))
		for _, obj := range objs {
			names = append(names, obj.GetName())
		}
		return names
	}

	if got, want := list(false), []string{"folder", "visible.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("show_hidden=false listed %q, want %q", got, want)
	}
	if got, want := list(true), []string{"$RECYCLE.BIN", ".dotfile", "folder", "hidden.txt", "visible.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("show_hidden=true listed %q, want %q", got, want)
	}
}
