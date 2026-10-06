package strm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/model"
)

func TestResolveStarts(t *testing.T) {
	d := &Strm{}
	d.aliases = map[string][]string{"movies": {"/local/movies"}}
	d.autoFlatten = true
	d.singleRootKey = "movies"
	starts := d.resolveStarts("/")
	if len(starts) != 1 || starts[0].virtualDir != "/" || starts[0].realDir != "/local/movies" {
		t.Fatalf("flatten root got %+v", starts)
	}

	d2 := &Strm{}
	d2.aliases = map[string][]string{"a": {"/ra"}, "b": {"/rb"}}
	d2.autoFlatten = false
	if len(d2.resolveStarts("/")) != 2 {
		t.Fatalf("non-flatten root want 2")
	}
	sub := d2.resolveStarts("/a/sub")
	if len(sub) != 1 || sub[0].virtualDir != "/a/sub" || sub[0].realDir != "/ra/sub" {
		t.Fatalf("non-flatten sub got %+v", sub)
	}
}

func TestGenerateUnitsWritesAndProgress(t *testing.T) {
	tmp := t.TempDir()
	d := &Strm{}
	d.SaveStrmLocalPath = tmp
	d.EncodePath = true
	d.WithoutUrl = true
	d.normalizedPrefix = "/d"

	units := []strmDirUnit{
		{virtualDir: "/Movies", objs: []model.Obj{
			&model.Object{ID: "strm", Path: "/real/Movies/m.mkv", Name: "m.strm"},
		}},
	}
	var last float64
	d.generateUnits(context.Background(), units, SaveLocalUpdateMode, func(p float64) { last = p })
	if last != 100 {
		t.Fatalf("progress want 100 got %v", last)
	}
	if b, err := os.ReadFile(filepath.Join(tmp, "Movies", "m.strm")); err != nil || len(b) == 0 {
		t.Fatalf("strm not written: err=%v len=%d", err, len(b))
	}
}
