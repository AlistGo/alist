package pcloud

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	defaultClientID     = "DnONSzyJXpm"
	defaultClientSecret = "VKEnd3ze4jsKFGg8TJiznwFG8"
)

// Get API base URL
func (d *PCloud) getAPIURL() string {
	return "https://" + d.Hostname
}

// Get OAuth client credentials
func (d *PCloud) getClientCredentials() (string, string) {
	clientID := d.ClientID
	clientSecret := d.ClientSecret

	if clientID == "" {
		clientID = defaultClientID
	}
	if clientSecret == "" {
		clientSecret = defaultClientSecret
	}

	return clientID, clientSecret
}

// Refresh OAuth access token
func (d *PCloud) refreshToken() error {
	clientID, clientSecret := d.getClientCredentials()

	var resp TokenResponse
	_, err := base.RestyClient.R().
		SetFormData(map[string]string{
			"client_id":     clientID,
			"client_secret": clientSecret,
			"grant_type":    "refresh_token",
			"refresh_token": d.RefreshToken,
		}).
		SetResult(&resp).
		Post(d.getAPIURL() + "/oauth2_token")

	if err != nil {
		return err
	}

	d.AccessToken = resp.AccessToken
	return nil
}

// shouldRetry determines if an error should be retried based on pCloud-specific logic
func (d *PCloud) shouldRetry(statusCode int, apiError *ErrorResult) bool {
	// HTTP-level retry conditions
	if statusCode == 429 || statusCode >= 500 {
		return true
	}

	// pCloud API-specific retry conditions (like rclone)
	if apiError != nil && apiError.Result != 0 {
		// 4xxx: rate limiting
		if apiError.Result/1000 == 4 {
			return true
		}
		// 5xxx: internal errors
		if apiError.Result/1000 == 5 {
			return true
		}
	}

	return false
}

// requestWithRetry makes authenticated API request with retry logic
func (d *PCloud) requestWithRetry(endpoint string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	maxRetries := 3
	baseDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		body, err := d.request(endpoint, method, callback, resp)
		if err == nil {
			return body, nil
		}

		// If this is the last attempt, return the error
		if attempt == maxRetries {
			return nil, err
		}

		// Check if we should retry based on error type
		if !d.shouldRetryError(err) {
			return nil, err
		}

		// Exponential backoff
		delay := baseDelay * time.Duration(1<<attempt)
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// shouldRetryError checks if an error should trigger a retry
func (d *PCloud) shouldRetryError(err error) bool {
	// For now, we'll retry on any error
	// In production, you'd want more specific error handling
	return true
}

// Make authenticated API request
func (d *PCloud) request(endpoint string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()

	// Add access token as query parameter (pCloud doesn't use Bearer auth)
	req.SetQueryParam("access_token", d.AccessToken)

	if callback != nil {
		callback(req)
	}

	if resp != nil {
		req.SetResult(resp)
	}

	var res *resty.Response
	var err error

	switch method {
	case http.MethodGet:
		res, err = req.Get(d.getAPIURL() + endpoint)
	case http.MethodPost:
		res, err = req.Post(d.getAPIURL() + endpoint)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return nil, err
	}

	// Check for API errors with pCloud-specific logic
	if res.StatusCode() != 200 {
		var errResp ErrorResult
		if err := utils.Json.Unmarshal(res.Body(), &errResp); err == nil {
			// Check if this error should trigger a retry
			if d.shouldRetry(res.StatusCode(), &errResp) {
				return nil, fmt.Errorf("pCloud API error (retryable): %s (result: %d)", errResp.Error, errResp.Result)
			}
			return nil, fmt.Errorf("pCloud API error: %s (result: %d)", errResp.Error, errResp.Result)
		}
		return nil, fmt.Errorf("HTTP error: %d", res.StatusCode())
	}

	return res.Body(), nil
}

// List files in a folder
func (d *PCloud) getFiles(folderID string) ([]FileObject, error) {
	var resp ItemResult
	_, err := d.requestWithRetry("/listfolder", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("folderid", extractID(folderID))
	}, &resp)

	if err != nil {
		return nil, err
	}

	if resp.Result != 0 {
		return nil, fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	if resp.Metadata == nil {
		return []FileObject{}, nil
	}

	return resp.Metadata.Contents, nil
}

// Get download link for a file
func (d *PCloud) getDownloadLink(fileID string) (string, error) {
	var resp DownloadLinkResult
	_, err := d.requestWithRetry("/getfilelink", http.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("fileid", extractID(fileID))
	}, &resp)

	if err != nil {
		return "", err
	}

	if resp.Result != 0 {
		return "", fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	if len(resp.Hosts) == 0 {
		return "", fmt.Errorf("no download hosts available")
	}

	return "https://" + resp.Hosts[0] + resp.Path, nil
}

// Create a folder
func (d *PCloud) createFolder(parentID, name string) error {
	var resp ItemResult
	_, err := d.requestWithRetry("/createfolder", http.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"folderid": extractID(parentID),
			"name":     name,
		})
	}, &resp)

	if err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	return nil
}

// Delete a file or folder
func (d *PCloud) delete(objID string, isFolder bool) error {
	endpoint := "/deletefile"
	paramName := "fileid"

	if isFolder {
		endpoint = "/deletefolderrecursive"
		paramName = "folderid"
	}

	var resp ItemResult
	_, err := d.requestWithRetry(endpoint, http.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			paramName: extractID(objID),
		})
	}, &resp)

	if err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud error: result code %d", resp.Result)
	}

	return nil
}

// Upload a file using direct /uploadfile endpoint like rclone
func (d *PCloud) uploadFile(ctx context.Context, file io.Reader, parentID, name string, size int64) error {
	// pCloud requires Content-Length, so we need to know the size
	if size <= 0 {
		return fmt.Errorf("file size must be provided for pCloud upload")
	}

	// Upload directly to /uploadfile endpoint like rclone
	var resp ItemResult
	req := base.RestyClient.R().
		SetQueryParam("access_token", d.AccessToken).
		SetHeader("Content-Length", strconv.FormatInt(size, 10)).
		SetFileReader("content", name, file).
		SetFormData(map[string]string{
			"filename": name,
			"folderid": extractID(parentID),
			"nopartial": "1",
		})

	// Use PUT method like rclone
	res, err := req.Put(d.getAPIURL() + "/uploadfile")
	if err != nil {
		return err
	}

	// Parse response
	if err := utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return err
	}

	if resp.Result != 0 {
		return fmt.Errorf("pCloud upload error: result code %d", resp.Result)
	}

	return nil
}