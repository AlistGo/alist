package op_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/drivers/local"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
)

// decompressLocal is the Local driver with server-side archive decompression,
// which writes a fixed file into the destination directory.
type decompressLocal struct {
	local.Local
}

func (d *decompressLocal) Config() driver.Config {
	config := d.Local.Config()
	config.Name = "LocalDecompressTest"
	return config
}

func (d *decompressLocal) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) error {
	return os.WriteFile(filepath.Join(dstDir.GetPath(), "extracted.txt"), []byte("extracted"), 0o644)
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &decompressLocal{}
	})
}

// TestArchiveDecompressSameStorageCreatesMissingNestedDstDir covers the gap
// fixed for Copy and Move: decompressing within the same storage fetched
// dstDir without ensuring it exists, so it failed with "failed to get dst
// dir", while the task based path used for other storages goes through Put,
// which does create it.
func TestArchiveDecompressSameStorageCreatesMissingNestedDstDir(t *testing.T) {
	root := t.TempDir()
	mountPath := "/decompresstest-" + t.Name()
	_, err := op.CreateStorage(context.Background(), model.Storage{
		Driver:    "LocalDecompressTest",
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

	if err := os.WriteFile(filepath.Join(root, "src.zip"), []byte("archive"), 0o644); err != nil {
		t.Fatalf("failed to write source file: %+v", err)
	}

	// dstDirPath's intermediate directories ("a", "a/b") do not exist yet.
	err = op.ArchiveDecompress(context.Background(), storage, "/src.zip", "/a/b", model.ArchiveDecompressArgs{})
	if err != nil {
		t.Fatalf("ArchiveDecompress into a not-yet-existing nested destination failed: %+v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "a", "b", "extracted.txt"))
	if err != nil {
		t.Fatalf("extracted file not found at destination: %+v", err)
	}
	if string(got) != "extracted" {
		t.Errorf("extracted file content = %q, want %q", got, "extracted")
	}
}
