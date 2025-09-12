package mediafire

/*
Package mediafire
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-11

D@' 3z K!7 - The King Of Cracking
*/

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func (d *Mediafire) getSessionToken(ctx context.Context) (string, error) {
	tokenURL := d.hostBase + "/application/get_session_token.php"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Cookie", d.Cookie)
	req.Header.Set("DNT", "1")
	req.Header.Set("Origin", d.hostBase)
	req.Header.Set("Priority", "u=1, i")
	req.Header.Set("Referer", (d.hostBase + "/"))
	req.Header.Set("Sec-Ch-Ua", d.secChUa)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", d.secChUaPlatform)
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("User-Agent", d.userAgent)
	//req.Header.Set("Connection", "keep-alive")

	resp, err := base.HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	//fmt.Printf("getSessionToken :: Raw response: %s\n", string(body))
	//fmt.Printf("getSessionToken :: Parsed response: %+v\n", resp)

	var tokenResp struct {
		Response struct {
			SessionToken string `json:"session_token"`
		} `json:"response"`
	}

	if resp.StatusCode == 200 {
		if err := json.Unmarshal(body, &tokenResp); err != nil {
			return "", err
		}

		if tokenResp.Response.SessionToken == "" {
			return "", fmt.Errorf("empty session token received")
		}

		cookieMap := make(map[string]string)
		for _, cookie := range resp.Cookies() {
			cookieMap[cookie.Name] = cookie.Value
		}

		if len(cookieMap) > 0 {

			var cookies []string
			for name, value := range cookieMap {
				cookies = append(cookies, fmt.Sprintf("%s=%s", name, value))
			}
			d.Cookie = strings.Join(cookies, "; ")
			op.MustSaveDriverStorage(d)

			//fmt.Printf("getSessionToken :: Captured cookies: %s\n", d.Cookie)
		}

	} else {
		return "", fmt.Errorf("getSessionToken :: failed to get session token, status code: %d", resp.StatusCode)
	}

	d.SessionToken = tokenResp.Response.SessionToken

	//fmt.Printf("Init :: Obtain Session Token %v", d.SessionToken)

	op.MustSaveDriverStorage(d)

	return d.SessionToken, nil
}

func (d *Mediafire) renewToken(_ context.Context) error {
	query := map[string]string{
		"session_token":   d.SessionToken,
		"response_format": "json",
	}

	var resp MediafireRenewTokenResponse
	_, err := d.postForm("/user/renew_session_token.php", query, &resp)
	if err != nil {
		return fmt.Errorf("failed to renew token: %w", err)
	}

	//fmt.Printf("getInfo :: Raw response: %s\n", string(body))
	//fmt.Printf("getInfo :: Parsed response: %+v\n", resp)

	if resp.Response.Result != "Success" {
		return fmt.Errorf("MediaFire token renewal failed: %s", resp.Response.Result)
	}

	d.SessionToken = resp.Response.SessionToken

	//fmt.Printf("Init :: Renew Session Token: %s", resp.Response.Result)

	op.MustSaveDriverStorage(d)

	return nil
}

func (d *Mediafire) getFiles(ctx context.Context, folderKey string) ([]File, error) {
	files := make([]File, 0)
	hasMore := true
	chunkNumber := 1

	for hasMore {
		resp, err := d.getFolderContent(ctx, folderKey, chunkNumber)
		if err != nil {
			return nil, err
		}

		for _, folder := range resp.Folders {
			files = append(files, File{
				ID:         folder.FolderKey,
				Name:       folder.Name,
				Size:       0,
				CreatedUTC: folder.CreatedUTC,
				IsFolder:   true,
			})
		}

		for _, file := range resp.Files {
			size, _ := strconv.ParseInt(file.Size, 10, 64)
			files = append(files, File{
				ID:         file.QuickKey,
				Name:       file.Filename,
				Size:       size,
				CreatedUTC: file.CreatedUTC,
				IsFolder:   false,
			})
		}

		hasMore = resp.MoreChunks
		chunkNumber++
	}

	return files, nil
}

func (d *Mediafire) getFolderContent(ctx context.Context, folderKey string, chunkNumber int) (*FolderContentResponse, error) {

	foldersResp, err := d.getFolderContentByType(ctx, folderKey, "folders", chunkNumber)
	if err != nil {
		return nil, err
	}

	filesResp, err := d.getFolderContentByType(ctx, folderKey, "files", chunkNumber)
	if err != nil {
		return nil, err
	}

	return &FolderContentResponse{
		Folders:    foldersResp.Response.FolderContent.Folders,
		Files:      filesResp.Response.FolderContent.Files,
		MoreChunks: foldersResp.Response.FolderContent.MoreChunks == "yes" || filesResp.Response.FolderContent.MoreChunks == "yes",
	}, nil
}

func (d *Mediafire) getFolderContentByType(_ context.Context, folderKey, contentType string, chunkNumber int) (*MediafireResponse, error) {
	data := map[string]string{
		"session_token":   d.SessionToken,
		"response_format": "json",
		"folder_key":      folderKey,
		"content_type":    contentType,
		"chunk":           strconv.Itoa(chunkNumber),
		"chunk_size":      strconv.FormatInt(d.ChunkSize, 10),
		"details":         "yes",
		"order_direction": d.OrderDirection,
		"order_by":        d.OrderBy,
		"filter":          "",
	}

	var resp MediafireResponse
	_, err := d.postForm("/folder/get_content.php", data, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	return &resp, nil
}

func (d *Mediafire) fileToObj(f File) *model.ObjThumb {
	created, _ := time.Parse("2006-01-02T15:04:05Z", f.CreatedUTC)

	var thumbnailURL string
	if !f.IsFolder && f.ID != "" {
		thumbnailURL = d.hostBase + "/convkey/acaa/" + f.ID + "3g.jpg"
	}

	return &model.ObjThumb{
		Object: model.Object{
			ID: f.ID,
			//Path:     "",
			Name:     f.Name,
			Size:     f.Size,
			Modified: created,
			Ctime:    created,
			IsFolder: f.IsFolder,
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: thumbnailURL,
		},
	}
}

func (d *Mediafire) getForm(endpoint string, query map[string]string, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()

	req.SetQueryParams(query)

	req.SetHeaders(map[string]string{
		"Cookie": d.Cookie,
		//"User-Agent": base.UserAgent,
		"User-Agent": d.userAgent,
		"Origin":     d.appBase,
		"Referer":    d.appBase + "/",
	})

	// If response OK
	if resp != nil {
		req.SetResult(resp)
	}

	// Targets MediaFire API
	res, err := req.Get(d.apiBase + endpoint)
	if err != nil {
		return nil, err
	}

	return res.Body(), nil
}

func (d *Mediafire) postForm(endpoint string, data map[string]string, resp interface{}) ([]byte, error) {
	req := base.RestyClient.R()

	req.SetFormData(data)

	req.SetHeaders(map[string]string{
		"Cookie":       d.Cookie,
		"Content-Type": "application/x-www-form-urlencoded",
		//"User-Agent": base.UserAgent,
		"User-Agent": d.userAgent,
		"Origin":     d.appBase,
		"Referer":    d.appBase + "/",
	})

	// If response OK
	if resp != nil {
		req.SetResult(resp)
	}

	// Targets MediaFire API
	res, err := req.Post(d.apiBase + endpoint)
	if err != nil {
		return nil, err
	}

	return res.Body(), nil
}

func (d *Mediafire) getDirectDownloadLink(_ context.Context, fileID string) (string, error) {
	data := map[string]string{
		"session_token":   d.SessionToken,
		"quick_key":       fileID,
		"link_type":       "direct_download",
		"response_format": "json",
	}

	var resp MediafireDirectDownloadResponse
	_, err := d.getForm("/file/get_links.php", data, &resp)
	if err != nil {
		return "", err
	}

	if resp.Response.Result != "Success" {
		return "", fmt.Errorf("MediaFire API error: %s", resp.Response.Result)
	}

	if len(resp.Response.Links) == 0 {
		return "", fmt.Errorf("no download links found")
	}

	return resp.Response.Links[0].DirectDownload, nil
}

func (d *Mediafire) calculateSHA256(file *os.File) (string, error) {
	hasher := sha256.New()
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (d *Mediafire) uploadCheck(ctx context.Context, filename string, filesize int64, filehash, folderKey string) (*MediafireCheckResponse, error) {

	actionToken, err := d.getActionToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get action token: %w", err)
	}

	query := map[string]string{
		"session_token":   actionToken, /* d.SessionToken */
		"filename":        filename,
		"size":            strconv.FormatInt(filesize, 10),
		"hash":            filehash,
		"folder_key":      folderKey,
		"resumable":       "yes",
		"response_format": "json",
	}

	var resp MediafireCheckResponse
	_, err = d.postForm("/upload/check.php", query, &resp)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("uploadCheck :: Raw response: %s\n", string(body))
	//fmt.Printf("uploadCheck :: Parsed response: %+v\n", resp)

	//fmt.Printf("uploadCheck :: ResumableUpload section: %+v\n", resp.Response.ResumableUpload)
	//fmt.Printf("uploadCheck :: Upload key specifically: '%s'\n", resp.Response.ResumableUpload.UploadKey)

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire upload check failed: %s", resp.Response.Result)
	}

	return &resp, nil
}

func (d *Mediafire) resumableUpload(ctx context.Context, folderKey, uploadKey string, unitData []byte, unitID int, fileHash, filename string, totalFileSize int64) (string, error) {
	actionToken, err := d.getActionToken(ctx)
	if err != nil {
		return "", err
	}

	url := d.apiBase + "/upload/resumable.php"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(unitData))
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("folder_key", folderKey)
	q.Add("response_format", "json")
	q.Add("session_token", actionToken)
	q.Add("key", uploadKey)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("x-filehash", fileHash)
	req.Header.Set("x-filesize", strconv.FormatInt(totalFileSize, 10))
	req.Header.Set("x-unit-id", strconv.Itoa(unitID))
	req.Header.Set("x-unit-size", strconv.FormatInt(int64(len(unitData)), 10))
	req.Header.Set("x-unit-hash", d.sha256Hex(bytes.NewReader(unitData)))
	req.Header.Set("x-filename", filename)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(unitData))

	/* fmt.Printf("Debug resumable upload request:\n")
	fmt.Printf("  URL: %s\n", req.URL.String())
	fmt.Printf("  Headers: %+v\n", req.Header)
	fmt.Printf("  Unit ID: %d\n", unitID)
	fmt.Printf("  Unit Size: %d\n", len(unitData))
	fmt.Printf("  Upload Key: %s\n", uploadKey)
	fmt.Printf("  Action Token: %s\n", actionToken) */

	res, err := base.HttpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	//fmt.Printf("MediaFire resumable upload response (status %d): %s\n", res.StatusCode, string(body))

	var uploadResp struct {
		Response struct {
			Doupload struct {
				Key string `json:"key"`
			} `json:"doupload"`
			Result string `json:"result"`
		} `json:"response"`
	}

	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	if res.StatusCode != 200 {
		return "", fmt.Errorf("resumable upload failed with status %d", res.StatusCode)
	}

	return uploadResp.Response.Doupload.Key, nil
}

func (d *Mediafire) uploadUnits(ctx context.Context, file *os.File, checkResp *MediafireCheckResponse, filename, fileHash, folderKey string, up driver.UpdateProgress) (string, error) {
	unitSize, _ := strconv.ParseInt(checkResp.Response.ResumableUpload.UnitSize, 10, 64)
	numUnits, _ := strconv.Atoi(checkResp.Response.ResumableUpload.NumberOfUnits)
	uploadKey := checkResp.Response.ResumableUpload.UploadKey

	stringWords := checkResp.Response.ResumableUpload.Bitmap.Words
	intWords := make([]int, len(stringWords))
	for i, word := range stringWords {
		intWords[i], _ = strconv.Atoi(word)
	}

	var finalUploadKey string

	for unitID := 0; unitID < numUnits; unitID++ {

		if utils.IsCanceled(ctx) {
			return "", ctx.Err()
		}

		if d.isUnitUploaded(intWords, unitID) {
			up(float64(unitID+1) * 100 / float64(numUnits))
			continue
		}

		uploadKey, err := d.uploadSingleUnit(ctx, file, unitID, unitSize, fileHash, filename, uploadKey, folderKey)
		if err != nil {
			return "", err
		}

		finalUploadKey = uploadKey

		up(float64(unitID+1) * 100 / float64(numUnits))
	}

	return finalUploadKey, nil
}

func (d *Mediafire) uploadSingleUnit(ctx context.Context, file *os.File, unitID int, unitSize int64, fileHash, filename, uploadKey, folderKey string) (string, error) {
	start := int64(unitID) * unitSize
	size := unitSize

	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	fileSize := stat.Size()

	if start+size > fileSize {
		size = fileSize - start
	}

	unitData := make([]byte, size)
	if _, err := file.ReadAt(unitData, start); err != nil {
		return "", err
	}

	return d.resumableUpload(ctx, folderKey, uploadKey, unitData, unitID, fileHash, filename, fileSize)
}

func (d *Mediafire) getActionToken(_ context.Context) (string, error) {

	if d.actionToken != "" {
		return d.actionToken, nil
	}

	data := map[string]string{
		"type":            "upload",
		"lifespan":        "1440",
		"response_format": "json",
		"session_token":   d.SessionToken,
	}

	var resp MediafireActionTokenResponse
	_, err := d.postForm("/user/get_action_token.php", data, &resp)
	if err != nil {
		return "", err
	}

	if resp.Response.Result != "Success" {
		return "", fmt.Errorf("MediaFire action token failed: %s", resp.Response.Result)
	}

	return resp.Response.ActionToken, nil
}

func (d *Mediafire) pollUpload(ctx context.Context, key string) (*MediafirePollResponse, error) {

	actionToken, err := d.getActionToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get action token: %w", err)
	}

	//fmt.Printf("Debug Key: %+v\n", key)

	query := map[string]string{
		"key":             key,
		"response_format": "json",
		"session_token":   actionToken, /* d.SessionToken */
	}

	var resp MediafirePollResponse
	_, err = d.postForm("/upload/poll_upload.php", query, &resp)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("pollUpload :: Raw response: %s\n", string(body))
	//fmt.Printf("pollUpload :: Parsed response: %+v\n", resp)

	//fmt.Printf("pollUpload :: Debug Result: %+v\n", resp.Response.Result)

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire poll upload failed: %s", resp.Response.Result)
	}

	return &resp, nil
}

func (d *Mediafire) sha256Hex(r io.Reader) string {
	h := sha256.New()
	io.Copy(h, r)
	return hex.EncodeToString(h.Sum(nil))
}

func (d *Mediafire) isUnitUploaded(words []int, unitID int) bool {
	wordIndex := unitID / 16
	bitIndex := unitID % 16
	if wordIndex >= len(words) {
		return false
	}
	return (words[wordIndex]>>bitIndex)&1 == 1
}

func (d *Mediafire) getExistingFileInfo(ctx context.Context, fileHash, filename, folderKey string) (*model.ObjThumb, error) {

	if fileInfo, err := d.getFileByHash(ctx, fileHash); err == nil && fileInfo != nil {
		return fileInfo, nil
	}

	files, err := d.getFiles(ctx, folderKey)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.Name == filename && !file.IsFolder {
			return d.fileToObj(file), nil
		}
	}

	return nil, fmt.Errorf("existing file not found")
}

func (d *Mediafire) getFileByHash(_ context.Context, hash string) (*model.ObjThumb, error) {
	query := map[string]string{
		"session_token":   d.SessionToken,
		"response_format": "json",
		"hash":            hash,
	}

	var resp MediafireFileSearchResponse
	_, err := d.postForm("/file/get_info.php", query, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Response.Result != "Success" {
		return nil, fmt.Errorf("MediaFire file search failed: %s", resp.Response.Result)
	}

	if len(resp.Response.FileInfo) == 0 {
		return nil, fmt.Errorf("file not found by hash")
	}

	file := resp.Response.FileInfo[0]
	return d.fileToObj(file), nil
}
