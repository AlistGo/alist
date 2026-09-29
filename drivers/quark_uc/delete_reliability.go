package quark

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/cookie"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	deleteControlMaxAttempts    = 3
	deleteControlInitialBackoff = 250 * time.Millisecond
	deleteControlMaxBackoff     = 500 * time.Millisecond

	// quarkFileNotFoundCode is the provider code Quark returns from
	// /file/info for a FID that no longer exists. Observed against the real
	// provider as HTTP 404 with status 404 and this code; it is the only
	// signature that may be read as absence.
	quarkFileNotFoundCode = 21001
)

type deleteFileInfoResp struct {
	Status  *int   `json:"status"`
	Code    *int   `json:"code"`
	Message string `json:"message"`
	// Data is a pointer so a response that carries no data object at all can be
	// told apart from one that does. Only Fid is read: presence is decided by
	// exact FID equality and by nothing else the provider may return.
	Data *struct {
		Fid string `json:"fid"`
	} `json:"data"`
}

type deleteFileActionResp struct {
	Status  *int   `json:"status"`
	Code    *int   `json:"code"`
	Message string `json:"message"`
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

// isAmbiguousQuarkDeleteTransportError reports transport failures returned by
// net/http. removeReliable checks ctx.Err first, so caller cancellation is not
// reclassified as a retryable transport ambiguity.
func isAmbiguousQuarkDeleteTransportError(err error) bool {
	var urlErr *url.Error
	return errors.As(err, &urlErr)
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

// isQuarkFileNotFound reports whether err is the exact provider signature for a
// FID that no longer exists. All three fields must match: a 404 without the
// provider envelope, or the envelope without the HTTP status, is not proof of
// absence. Matching is structural; the message text is never inspected.
func isQuarkFileNotFound(err error) bool {
	var provErr *providerError
	if !errors.As(err, &provErr) {
		return false
	}
	return provErr.HTTPStatus == http.StatusNotFound &&
		provErr.Status == http.StatusNotFound &&
		provErr.Code == quarkFileNotFoundCode
}

func (d *QuarkOrUC) deleteControlSourceClient() *resty.Client {
	if d.client != nil {
		return d.client
	}
	return base.RestyClient
}

// deleteControlNoRetryClient wraps the same concurrency-safe net/http client as
// the normal Resty client but has an independent Resty retry policy. This keeps
// transport/TLS/timeout behavior while preventing Resty from replaying a
// destructive /file/delete before removeReliable can verify the FID.
func (d *QuarkOrUC) deleteControlNoRetryClient() *resty.Client {
	source := d.deleteControlSourceClient()
	client := resty.NewWithClient(source.GetClient()).SetRetryCount(0)
	client.Header = source.Header.Clone()
	return client
}

func (d *QuarkOrUC) mergeDeleteControlResponseCookies(res *resty.Response) {
	if res == nil {
		return
	}
	var updated bool
	d.cookieMu.Lock()
	__puus := cookie.GetCookie(res.Cookies(), "__puus")
	if __puus != nil {
		d.Cookie = cookie.SetStr(d.Cookie, "__puus", __puus.Value)
		updated = true
	}
	if d.UseTransCodingAddress && d.config.Name == "Quark" {
		__pus := cookie.GetCookie(res.Cookies(), "__pus")
		if __pus != nil {
			d.Cookie = cookie.SetStr(d.Cookie, "__pus", __pus.Value)
			updated = true
		}
	}
	d.cookieMu.Unlock()
	if updated {
		op.MustSaveDriverStorage(d)
	}
}

// deleteControlRequest is the narrow request path used only by delete
// reliability. It preserves the existing Quark headers, query parameters and
// cookie-refresh behavior, exposes the actual HTTP status, and can disable
// Resty's client-level retries for destructive requests without mutating the
// shared client.
func (d *QuarkOrUC) deleteControlRequest(
	ctx context.Context,
	pathname string,
	method string,
	body interface{},
	query map[string]string,
	result interface{},
	noRetry bool,
) (int, error) {
	client := d.deleteControlSourceClient()
	if noRetry {
		client = d.deleteControlNoRetryClient()
	}

	d.cookieMu.Lock()
	cookieStr := d.Cookie
	d.cookieMu.Unlock()

	req := client.R()
	req.SetHeaders(map[string]string{
		"Cookie":  cookieStr,
		"Accept":  "application/json, text/plain, */*",
		"Referer": d.conf.referer,
	})
	req.SetQueryParam("pr", d.conf.pr)
	req.SetQueryParam("fr", "pc")
	if query != nil {
		req.SetQueryParams(query)
	}
	req.SetContext(ctx)
	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}
	var providerResp Resp
	req.SetError(&providerResp)

	res, err := req.Execute(method, d.conf.api+pathname)
	if res != nil {
		d.mergeDeleteControlResponseCookies(res)
	}
	if err != nil {
		if res != nil {
			return res.StatusCode(), err
		}
		return 0, err
	}

	httpStatus := res.StatusCode()
	if httpStatus >= http.StatusBadRequest || providerResp.Status >= 400 || providerResp.Code != 0 {
		return httpStatus, &providerError{
			HTTPStatus: httpStatus,
			Status:     providerResp.Status,
			Code:       providerResp.Code,
			Message:    providerResp.Message,
		}
	}
	return httpStatus, nil
}

func (d *QuarkOrUC) deleteFileOnce(ctx context.Context, data base.Json) error {
	var resp deleteFileActionResp
	httpStatus, err := d.deleteControlRequest(ctx, "/file/delete", http.MethodPost, data, nil, &resp, true)
	if err != nil {
		return err
	}
	if httpStatus != http.StatusOK {
		return fmt.Errorf("quark delete returned unexpected http status=%d", httpStatus)
	}
	if resp.Status == nil || resp.Code == nil {
		return errors.New("quark delete returned incomplete provider envelope")
	}
	if *resp.Status != http.StatusOK || *resp.Code != 0 {
		return &providerError{
			HTTPStatus: httpStatus,
			Status:     *resp.Status,
			Code:       *resp.Code,
			Message:    resp.Message,
		}
	}
	return nil
}

// deleteFileExistsByFID resolves a single FID through /file/info, the endpoint
// that answers per-FID existence directly. Presence requires HTTP 200 and a
// successful provider envelope carrying a data object whose fid equals the
// requested one; absence requires the exact not-found signature. Everything
// else — an invalid-FID rejection, an auth failure, a rate limit, a gateway
// page, malformed JSON, a missing data object, or a mismatched fid — is an
// error, never absence.
func (d *QuarkOrUC) deleteFileExistsByFID(ctx context.Context, fid string) (bool, error) {
	if fid == "" {
		return false, errors.New("quark file info requires a non-empty fid")
	}
	var resp deleteFileInfoResp
	httpStatus, err := d.deleteControlRequest(
		ctx,
		"/file/info",
		http.MethodGet,
		nil,
		map[string]string{"fid": fid},
		&resp,
		false,
	)
	if err != nil {
		if isQuarkFileNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if httpStatus != http.StatusOK {
		return false, fmt.Errorf("quark file info returned unexpected http status=%d for fid=%s", httpStatus, fid)
	}
	if resp.Code == nil || resp.Status == nil {
		return false, fmt.Errorf("quark file info returned incomplete envelope for fid=%s", fid)
	}
	if *resp.Code != 0 || *resp.Status != http.StatusOK {
		return false, fmt.Errorf("quark file info rejected for fid=%s: status=%d code=%d message=%s",
			fid, *resp.Status, *resp.Code, resp.Message)
	}
	if resp.Data == nil {
		return false, fmt.Errorf("quark file info returned no data for fid=%s", fid)
	}
	if resp.Data.Fid != fid {
		return false, fmt.Errorf("quark file info returned fid=%s for requested fid=%s", resp.Data.Fid, fid)
	}
	return true, nil
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

		err := d.deleteFileOnce(ctx, data)
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		providerRetryable := isRetryableQuarkDeleteError(err)
		transportAmbiguous := isAmbiguousQuarkDeleteTransportError(err)
		retryable := providerRetryable || transportAmbiguous
		if retryable {
			hadTransient = true
		}

		// A retryable response or transport failure is ambiguous: Quark may have
		// accepted the delete before returning the error. Verify the immutable FID
		// before replaying the destructive request. After an earlier ambiguity,
		// also verify a later non-retryable response so an already-completed
		// delete is not turned back into a failure.
		if retryable || hadTransient {
			exists, verifyErr := d.deleteFileExistsByFID(ctx, fid)
			if verifyErr == nil && !exists {
				log.Warnf("quark delete returned an error but fid=%s is absent; treating delete as success: %v", fid, err)
				return nil
			}
			if verifyErr != nil {
				log.Warnf("quark delete fid verification failed attempt=%d/%d fid=%s: %v", attempt, deleteControlMaxAttempts, fid, verifyErr)
				// A transport failure is ambiguous specifically because the request may
				// have reached the provider. Without a successful verifier result, a
				// replay would be unsafe, so fail closed instead.
				if transportAmbiguous {
					return fmt.Errorf("quark delete transport outcome is ambiguous and fid verification failed: %w", verifyErr)
				}
			}
		}

		if !retryable {
			return err
		}
		if attempt == deleteControlMaxAttempts {
			return fmt.Errorf("quark delete transient provider error after %d attempts: %w", attempt, err)
		}

		log.Warnf("quark delete ambiguous error attempt=%d/%d fid=%s: %v; retrying", attempt, deleteControlMaxAttempts, fid, err)
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
