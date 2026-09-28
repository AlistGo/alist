package op_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
)

// TestCopySameStorageCreatesMissingNestedDstDir is a regression test for
// https://github.com/AlistGo/alist/issues/8394: Copy fetched dstDir without
// first ensuring it exists, so copying within the same storage into a
// destination whose intermediate directories don't exist yet failed with
// "failed to get dst dir", while the equivalent cross-storage copy (which
// goes through Put, which does call MakeDir first) succeeded.
func TestCopySameStorageCreatesMissingNestedDstDir(t *testing.T) {
	root := t.TempDir()
	mountPath := "/copytest-" + t.Name()
	_, err := op.CreateStorage(context.Background(), model.Storage{
		Driver:    "Local",
		MountPath: mountPath,
		Addition:  fmt.Sprintf(`{"root_folder_path":%q}`, root),
	})
	if err != nil {
		t.Fatalf("failed to create storage: %+v", err)
	}
	storage, err := op.GetStorageByMountPath(mountPath)
	if err != nil {
		t.Fatalf("failed to get storage: %+v", err)
	}

	const content = "hello"
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write source file: %+v", err)
	}

	// dstDirPath's intermediate directories ("a", "a/b") do not exist yet.
	err = op.Copy(context.Background(), storage, "/src.txt", "/a/b")
	if err != nil {
		t.Fatalf("Copy into a not-yet-existing nested destination failed: %+v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "a", "b", "src.txt"))
	if err != nil {
		t.Fatalf("copied file not found at destination: %+v", err)
	}
	if string(got) != content {
		t.Errorf("copied file content = %q, want %q", got, content)
	}
}
