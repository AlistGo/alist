package azure_blob

import "github.com/alist-org/alist/v3/internal/driver"

// progressTracker is used to track upload progress
type progressTracker struct {
	total          int64
	current        int64
	updateProgress driver.UpdateProgress
}

// Write implements io.Writer to track progress
func (pt *progressTracker) Write(p []byte) (n int, err error) {
	n = len(p)
	pt.current += int64(n)
	if pt.updateProgress != nil && pt.total > 0 {
		pt.updateProgress(float64(pt.current) * 100 / float64(pt.total))
	}
	return n, nil
}
