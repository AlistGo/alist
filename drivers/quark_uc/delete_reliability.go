package quark

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	deleteControlMaxAttempts    = 3
	deleteControlInitialBackoff = 250 * time.Millisecond
	deleteControlMaxBackoff     = 500 * time.Millisecond
)

type deleteFileInfoResp struct {
	Resp
	Data struct {
		// List is a pointer so an explicit empty list can be told apart from a
		// response that carries no list at all.
		List *[]File `json:"list"`
	} `json:"data"`
}

func hasQuarkDeleteErrorTokenPrefix(msg, token string) bool {
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

func isRetryableQuarkDeleteError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return hasQuarkDeleteErrorTokenPrefix(msg, "inner error") && strings.Contains(msg, "requestid")
}

func waitDeleteRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (d *QuarkOrUC) deleteFileExistsByFID(ctx context.Context, fid string) (bool, error) {
	var resp deleteFileInfoResp
	_, err := d.request("/file", http.MethodGet, func(req *resty.Request) {
		req.SetContext(ctx).SetQueryParam("fids", fid)
	}, &resp)
	if err != nil {
		return false, err
	}
	// request only reports errors it can read from the HTTP status line, so a
	// non-JSON gateway page, a 204, or an application-level error delivered with
	// HTTP 200 all arrive here as a zero-valued response. Absence may only be
	// derived from a well-formed file query, never from a response that merely
	// failed to produce a list.
	if resp.Code != 0 || resp.Status >= 400 {
		return false, fmt.Errorf("quark file query rejected for fid=%s: status=%d code=%d message=%s",
			fid, resp.Status, resp.Code, resp.Message)
	}
	if resp.Data.List == nil {
		return false, fmt.Errorf("quark file query returned no list for fid=%s", fid)
	}
	for _, file := range *resp.Data.List {
		if file.Fid == fid {
			return true, nil
		}
	}
	return false, nil
}

func (d *QuarkOrUC) removeReliable(ctx context.Context, obj model.Obj) error {
	fid := obj.GetID()
	data := base.Json{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     []string{fid},
	}

	backoff := deleteControlInitialBackoff
	hadTransient := false
	for attempt := 1; attempt <= deleteControlMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, err := d.request("/file/delete", http.MethodPost, func(req *resty.Request) {
			req.SetContext(ctx).SetBody(data)
		}, nil)
		if err == nil {
			return nil
		}

		retryable := isRetryableQuarkDeleteError(err)
		if retryable {
			hadTransient = true
		}

		// A retryable response is ambiguous: Quark may have accepted the delete
		// before returning the provider error. Verify the immutable FID directly
		// before replaying the destructive request. After an earlier transient,
		// also verify a later non-retryable response so an already-completed
		// delete is not turned back into a failure.
		if retryable || hadTransient {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			exists, verifyErr := d.deleteFileExistsByFID(ctx, fid)
			if verifyErr == nil && !exists {
				log.Warnf("quark delete returned an error but fid=%s is absent; treating delete as success: %v", fid, err)
				return nil
			}
			if verifyErr != nil {
				log.Warnf("quark delete fid verification failed attempt=%d/%d fid=%s: %v", attempt, deleteControlMaxAttempts, fid, verifyErr)
			}
		}

		if !retryable {
			return err
		}
		if attempt == deleteControlMaxAttempts {
			return fmt.Errorf("quark delete transient provider error after %d attempts: %w", attempt, err)
		}

		log.Warnf("quark delete transient provider error attempt=%d/%d fid=%s: %v; retrying", attempt, deleteControlMaxAttempts, fid, err)
		if err := waitDeleteRetry(ctx, backoff); err != nil {
			return err
		}
		backoff *= 2
		if backoff > deleteControlMaxBackoff {
			backoff = deleteControlMaxBackoff
		}
	}
	return fmt.Errorf("quark delete retry loop exhausted unexpectedly")
}
