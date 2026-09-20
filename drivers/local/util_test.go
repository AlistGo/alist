package local

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
)

func TestThumbnailCacheTracksRenderedWidth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 800, 400))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d := &Local{Addition: Addition{ThumbCacheFolder: dir}, thumbSize: 144, thumbPixel: 320}
	obj := &model.Object{Name: "source.png", Path: path}
	cachePaths := make(map[string]bool)
	for _, width := range []int{320, 640} {
		d.thumbPixel = width
		buf, cached, err := d.getThumb(obj)
		if err != nil {
			t.Fatal(err)
		}
		if buf == nil || cached != nil {
			t.Fatalf("width %d reused an old thumbnail", width)
		}
		cfg, _, err := image.DecodeConfig(buf)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != width || cfg.Height != width/2 {
			t.Fatalf("unexpected dimensions: %+v", cfg)
		}
		// Changing the UI thumbnail size must not invalidate the image cache.
		d.thumbSize++
		buf, cached, err = d.getThumb(obj)
		if err != nil {
			t.Fatal(err)
		}
		if buf != nil || cached == nil {
			t.Fatal("expected a cache hit for the same rendered width")
		}
		if cachePaths[*cached] {
			t.Fatal("different rendered widths shared a cache file")
		}
		cachePaths[*cached] = true
	}
}
