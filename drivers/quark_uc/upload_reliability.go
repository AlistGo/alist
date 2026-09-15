package quark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	uploadControlMaxAttempts             = 3
	uploadControlInitialBackoff          = time.Second
	uploadControlMaxBackoff              = 2 * time.Second
	uploadFinishVisibilityAttempts       = 4
	uploadFinishVisibilityInitialBackoff = 250 * time.Millisecond
	uploadFinishVisibilityMaxBackoff     = time.Second
	uploadFinishVisibilityMaxPages       = 1000
)

func wrapQuarkUploadStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("quark upload stage %s: %w", stage, err)
}

func hasQuarkUploadErrorTokenPrefix(msg, token string) bool {
	if msg == token {
		return true
	}
	if !strings.HasPrefix(msg, token) || len(msg) == len(token) {
		return false
	}
	switch msg[len(token)] {
	case ' ', ',', ':', '\t':
		return true
	default:
		return false
	}
}

func isRetryableQuarkUploadControlError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if hasQuarkUploadErrorTokenPrefix(msg, "complete_upload_lock_timeout") {
		return true
	}
	return hasQuarkUploadErrorTokenPrefix(msg, "inner error") && strings.Contains(msg, "requestid")
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryQuarkUploadControl[T any](ctx context.Context, stage string, fn func() (T, error)) (T, error) {
	var zero T
	backoff := uploadControlInitialBackoff
	for attempt := 1; attempt <= uploadControlMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, wrapQuarkUploadStage(stage, err)
		}

		value, err := fn()
		if err == nil {
			return value, nil
		}
		if !isRetryableQuarkUploadControlError(err) {
			return zero, wrapQuarkUploadStage(stage, err)
		}
		if attempt == uploadControlMaxAttempts {
			return zero, wrapQuarkUploadStage(stage, fmt.Errorf("transient provider error after %d attempts: %w", attempt, err))
		}

		log.Warnf("quark upload stage=%s transient provider error attempt=%d/%d: %v; retrying", stage, attempt, uploadControlMaxAttempts, err)
		if err := waitWithContext(ctx, backoff); err != nil {
			return zero, wrapQuarkUploadStage(stage, err)
		}
		backoff *= 2
		if backoff > uploadControlMaxBackoff {
			backoff = uploadControlMaxBackoff
		}
	}
	return zero, wrapQuarkUploadStage(stage, fmt.Errorf("retry loop exhausted unexpectedly"))
}

// upPreReliable intentionally does not add a driver-layer retry: replaying
// /file/upload/pre can allocate a second upload task/FID. It only adds
// cancellation and stage attribution.
func (d *QuarkOrUC) upPreReliable(ctx context.Context, file model.FileStreamer, parentID string) (UpPreResp, error) {
	if err := ctx.Err(); err != nil {
		return UpPreResp{}, wrapQuarkUploadStage("pre", err)
	}
	pre, err := d.upPre(file, parentID)
	if err != nil {
		return pre, wrapQuarkUploadStage("pre", err)
	}
	return pre, nil
}

func (d *QuarkOrUC) upHashReliable(ctx context.Context, md5, sha1, taskID string) (bool, error) {
	return retryQuarkUploadControl(ctx, "hash", func() (bool, error) {
		return d.upHash(md5, sha1, taskID)
	})
}

func (d *QuarkOrUC) upPartReliable(ctx context.Context, pre UpPreResp, mimeType string, partNumber int, part []byte) (string, error) {
	stage := fmt.Sprintf("part[%d]", partNumber)
	return retryQuarkUploadControl(ctx, stage, func() (string, error) {
		// Rebuild the reader for every attempt. Retries are admitted only for the
		// Quark /file/upload/auth control-plane transient, before this OSS body is sent.
		reader := driver.NewLimitedUploadStream(ctx, bytes.NewReader(part))
		return d.upPart(ctx, pre, mimeType, partNumber, io.Reader(reader))
	})
}

func (d *QuarkOrUC) upCommitReliable(ctx context.Context, pre UpPreResp, md5s []string) error {
	// upCommit asks Quark /file/upload/auth before issuing CompleteMultipartUpload.
	// The driver-level retry predicate accepts only the raw Quark control-plane
	// transient messages; OSS-formatted errors are not retried by this wrapper.
	_, err := retryQuarkUploadControl(ctx, "commit", func() (struct{}, error) {
		return struct{}{}, d.upCommit(pre, md5s)
	})
	return err
}

func (d *QuarkOrUC) upFinishReliable(ctx context.Context, pre UpPreResp, parentID, fileName string, fileSize int64) error {
	backoff := uploadControlInitialBackoff
	for attempt := 1; attempt <= uploadControlMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return wrapQuarkUploadStage("finish", err)
		}

		err := d.upFinish(pre)
		if err == nil {
			return nil
		}
		if !isRetryableQuarkUploadControlError(err) {
			return wrapQuarkUploadStage("finish", err)
		}

		if attempt < uploadControlMaxAttempts {
			log.Warnf("quark upload stage=finish transient provider error attempt=%d/%d: %v; retrying finish", attempt, uploadControlMaxAttempts, err)
			if err := waitWithContext(ctx, backoff); err != nil {
				return wrapQuarkUploadStage("finish", err)
			}
			backoff *= 2
			if backoff > uploadControlMaxBackoff {
				backoff = uploadControlMaxBackoff
			}
			continue
		}

		// Only use directory visibility as a last-resort tiebreaker after the
		// bounded finish retry budget is exhausted. A pre-allocated FID may exist
		// before finish, so checking it earlier could turn an uncertain finish
		// into a false success and let op.Put discard the old overwrite fallback.
		if pre.Data.Fid != "" {
			visible, visibilityErr := d.waitUploadedObjectVisible(ctx, pre, parentID)
			if visible {
				log.Warnf("quark upload stage=finish remained transient after %d attempts but uploaded object fid=%s is visible; treating as success: %v", uploadControlMaxAttempts, pre.Data.Fid, err)
				return nil
			}
			if visibilityErr != nil {
				log.Warnf("quark upload stage=finish visibility verification failed after retry budget: %v", visibilityErr)
			}
		} else {
			// fileName/fileSize are diagnostic only. They must never be used as an
			// identity fallback because an older object may share both values.
			log.Warnf("quark upload stage=finish cannot verify uploaded object name=%q size=%d because pre response has no fid; failing closed after retry budget: %v", fileName, fileSize, err)
		}
		return wrapQuarkUploadStage("finish", fmt.Errorf("transient provider error after %d attempts: %w", attempt, err))
	}
	return wrapQuarkUploadStage("finish", fmt.Errorf("retry loop exhausted unexpectedly"))
}

func (d *QuarkOrUC) waitUploadedObjectVisible(ctx context.Context, pre UpPreResp, parentID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if pre.Data.Fid == "" {
		return false, nil
	}

	backoff := uploadFinishVisibilityInitialBackoff
	var lastErr error
	for attempt := 0; attempt < uploadFinishVisibilityAttempts; attempt++ {
		visible, err := d.findUploadedFileByFID(ctx, parentID, pre.Data.Fid)
		if err != nil {
			lastErr = err
		} else {
			lastErr = nil
			if visible {
				return true, nil
			}
		}

		if attempt == uploadFinishVisibilityAttempts-1 {
			break
		}
		if err := waitWithContext(ctx, backoff); err != nil {
			return false, err
		}
		backoff *= 2
		if backoff > uploadFinishVisibilityMaxBackoff {
			backoff = uploadFinishVisibilityMaxBackoff
		}
	}
	return false, lastErr
}

func (d *QuarkOrUC) findUploadedFileByFID(ctx context.Context, parentID, fid string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if fid == "" {
		return false, nil
	}

	const pageSize = 100
	query := map[string]string{
		"pdir_fid":             parentID,
		"_size":                strconv.Itoa(pageSize),
		"_fetch_total":         "1",
		"fetch_all_file":       "1",
		"fetch_risk_file_name": "1",
	}
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if page > uploadFinishVisibilityMaxPages {
			return false, fmt.Errorf("quark upload visibility listing exceeded %d pages for parent %s", uploadFinishVisibilityMaxPages, parentID)
		}

		query["_page"] = strconv.Itoa(page)
		var resp SortResp
		_, err := d.request("/file/sort", http.MethodGet, func(req *resty.Request) {
			req.SetContext(ctx).SetQueryParams(query)
		}, &resp)
		if err != nil {
			return false, err
		}

		for i := range resp.Data.List {
			file := &resp.Data.List[i]
			if file.File && file.Fid == fid {
				return true, nil
			}
		}

		if resp.Metadata.Total > 0 {
			if page*pageSize >= resp.Metadata.Total {
				return false, nil
			}
		} else if len(resp.Data.List) < pageSize {
			return false, nil
		}
	}
}
