package driver

import "context"

// StrmGenerator is implemented by the Strm driver to (re)generate local .strm
// files for a subtree, reporting progress (0-100) via up.
type StrmGenerator interface {
	// GenerateLocal walks virtualPath (relative to the storage root, e.g. "/" or
	// "/Movies") and writes local files, reporting progress in percent.
	GenerateLocal(ctx context.Context, virtualPath string, up func(percent float64)) error
}
