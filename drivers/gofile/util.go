package gofile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	log "github.com/sirupsen/logrus"
)

const (
	baseAPI   = "https://api.gofile.io"
	uploadAPI = "https://upload.gofile.io"
)

func (d *Gofile) request(ctx context.Context, method, endpoint string, body io.Reader, headers map[string]string) (*http.Response, error) {
	var url string
	if strings.HasPrefix(endpoint, "http") {
		url = endpoint
	} else {
		url = baseAPI + endpoint
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+d.APIToken)
	req.Header.Set("User-Agent", "AList/3.0")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return base.HttpClient.Do(req)
}

func (d *Gofile) getJSON(ctx context.Context, endpoint string, result interface{}) error {
	resp, err := d.request(ctx, "GET", endpoint, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.handleError(resp)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (d *Gofile) postJSON(ctx context.Context, endpoint string, data interface{}, result interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := d.request(ctx, "POST", endpoint, bytes.NewBuffer(jsonData), headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.handleError(resp)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (d *Gofile) putJSON(ctx context.Context, endpoint string, data interface{}, result interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := d.request(ctx, "PUT", endpoint, bytes.NewBuffer(jsonData), headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.handleError(resp)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (d *Gofile) deleteJSON(ctx context.Context, endpoint string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := d.request(ctx, "DELETE", endpoint, bytes.NewBuffer(jsonData), headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return d.handleError(resp)
	}

	return nil
}

func (d *Gofile) handleError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	log.Debugf("Gofile API error (HTTP %d): %s", resp.StatusCode, string(body))

	var errorResp ErrorResponse
	if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Status == "error" {
		return fmt.Errorf("gofile API error: %s (code: %s)", errorResp.Error.Message, errorResp.Error.Code)
	}

	return fmt.Errorf("gofile API error: HTTP %d - %s", resp.StatusCode, string(body))
}

func (d *Gofile) uploadFile(ctx context.Context, folderId string, file model.FileStreamer, up driver.UpdateProgress) (*UploadResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if folderId != "" {
		writer.WriteField("folderId", folderId)
	}

	part, err := writer.CreateFormFile("file", filepath.Base(file.GetName()))
	if err != nil {
		return nil, err
	}

	// Copy with progress tracking if available
	if up != nil {
		reader := &progressReader{
			reader: file,
			total:  file.GetSize(),
			up:     up,
		}
		_, err = io.Copy(part, reader)
	} else {
		_, err = io.Copy(part, file)
	}

	if err != nil {
		return nil, err
	}

	writer.Close()

	headers := map[string]string{
		"Content-Type": writer.FormDataContentType(),
	}

	resp, err := d.request(ctx, "POST", uploadAPI+"/uploadfile", &body, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, d.handleError(resp)
	}

	var result UploadResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	return &result, err
}

func (d *Gofile) createDirectLink(ctx context.Context, contentId string) (string, error) {
	data := map[string]interface{}{}

	if d.DirectLinkExpiry > 0 {
		expireTime := time.Now().Add(time.Duration(d.DirectLinkExpiry) * time.Hour).Unix()
		data["expireTime"] = expireTime
	}

	var result DirectLinkResponse
	err := d.postJSON(ctx, fmt.Sprintf("/contents/%s/directlinks", contentId), data, &result)
	if err != nil {
		return "", err
	}

	return result.Data.DirectLink, nil
}

func (d *Gofile) convertContentToObj(content Content) model.Obj {
	return &model.ObjThumb{
		Object: model.Object{
			ID:       content.ID,
			Name:     content.Name,
			Size:     content.Size,
			Modified: content.ModifiedTime(),
			IsFolder: content.IsDir(),
		},
	}
}

func (d *Gofile) getAccountId(ctx context.Context) (string, error) {
	var result AccountResponse
	err := d.getJSON(ctx, "/accounts/getid", &result)
	if err != nil {
		return "", err
	}
	return result.Data.ID, nil
}

func (d *Gofile) getAccountInfo(ctx context.Context, accountId string) (*AccountInfoResponse, error) {
	var result AccountInfoResponse
	err := d.getJSON(ctx, fmt.Sprintf("/accounts/%s", accountId), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// progressReader wraps an io.Reader to track upload progress
type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	up     driver.UpdateProgress
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	pr.read += int64(n)
	if pr.up != nil && pr.total > 0 {
		progress := float64(pr.read) * 100 / float64(pr.total)
		pr.up(progress)
	}
	return n, err
}
