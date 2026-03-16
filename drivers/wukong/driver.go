package wukong

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/go-resty/resty/v2"
)

const (
	wukongBaseURL          = "https://api.wkbrowser.com"
	webReferer             = "https://pan.wkbrowser.com/"
	vodBaseURL             = "https://vod.bytedanceapi.com"
	vodRegion              = "cn-north-1"
	vodService             = "vod"
	videoSpaceName         = "wukong_netdisk_ugc"
	minUploadSubmitSuccess = 2000
	multipartChunkSize     = int64(5 * 1024 * 1024)
)

type Wukong struct {
	model.Storage
	Addition
	client *resty.Client
}

func (d *Wukong) Config() driver.Config {
	return config
}

func (d *Wukong) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Wukong) Init(ctx context.Context) error {
	d.client = base.NewRestyClient().
		SetBaseURL(wukongBaseURL).
		SetHeader("accept", "application/json, text/plain, */*").
		SetHeader("content-type", "application/json").
		SetHeader("referer", webReferer).
		SetHeader("origin", "https://pan.wkbrowser.com")
	if d.Cookie != "" {
		d.client.SetHeader("cookie", d.Cookie)
	}
	if d.RootFolderID == "" {
		d.RootFolderID = "0"
	}
	if strings.TrimSpace(d.Aid) == "" {
		d.Aid = "590353"
	}
	if strings.TrimSpace(d.Language) == "" {
		d.Language = "zh"
	}
	if d.PageSize <= 0 {
		d.PageSize = 100
	}
	return nil
}

func (d *Wukong) Drop(ctx context.Context) error {
	return nil
}

func (d *Wukong) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	fatherID := dir.GetID()
	if fatherID == "" {
		fatherID = d.RootFolderID
	}
	offset := 0
	limit := d.PageSize
	objs := make([]model.Obj, 0)
	for {
		var resp filterFileResp
		_, err := d.client.R().
			SetContext(ctx).
			SetQueryParams(map[string]string{
				"offset":          strconv.Itoa(offset),
				"limit":           strconv.Itoa(limit),
				"aid":             d.Aid,
				"device_platform": "web",
				"language":        d.Language,
			}).
			SetBody(map[string]any{
				"father_id":   asIDValue(fatherID),
				"filter_type": 2,
				"is_desc":     1,
				"file_type":   0,
			}).
			SetResult(&resp).
			Post("/netdisk/user_file/filter_file")
		if err != nil {
			return nil, err
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("wukong list failed: code=%d message=%s", resp.Code, resp.Message)
		}

		for _, item := range resp.Data.FileList {
			objs = append(objs, &model.Object{
				ID:       strconv.FormatInt(item.FileID, 10),
				Path:     strconv.FormatInt(item.FatherID, 10),
				Name:     item.FileName,
				Size:     item.Size,
				Modified: parseUnix(item.UpdatedAt),
				Ctime:    parseUnix(item.CreatedAt),
				IsFolder: item.IsDirectory == 1,
			})
		}

		if !hasMore(resp.Data.HasMore) || len(resp.Data.FileList) == 0 {
			break
		}
		offset += len(resp.Data.FileList)
	}
	return objs, nil
}

func (d *Wukong) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.NotFile
	}
	fileID := file.GetID()
	if fileID == "" {
		return nil, errors.New("missing file id")
	}

	var resp rawResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(map[string]any{
			"file_id_list": []any{asIDValue(fileID)},
		}).
		SetResult(&resp).
		Post("/netdisk/user_file/detail")
	if err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("wukong detail failed: code=%d message=%s", resp.Code, resp.Message)
	}

	url := extractDetailMainURL(resp.Data)
	if url == "" {
		url = extractURL(resp.Data)
	}
	if url == "" {
		return nil, errs.NotImplement
	}

	return &model.Link{
		URL: url,
		Header: http.Header{
			"Referer": []string{webReferer},
			"Cookie":  []string{d.Cookie},
		},
	}, nil
}

func (d *Wukong) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	fatherID := parentDir.GetID()
	if fatherID == "" {
		fatherID = d.RootFolderID
	}

	var resp rawResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(map[string]any{
			"father_id": asIDValue(fatherID),
			"file_name": dirName,
		}).
		SetResult(&resp).
		Post("/netdisk/user_file/create_directory")
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("wukong create directory failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func (d *Wukong) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcID := srcObj.GetID()
	if srcID == "" {
		return errors.New("missing source file id")
	}

	dstID := dstDir.GetID()
	if dstID == "" {
		dstID = d.RootFolderID
	}

	var resp rawResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(map[string]any{
			"file_id_list":  []any{asIDValue(srcID)},
			"new_father_id": asIDValue(dstID),
		}).
		SetResult(&resp).
		Post("/netdisk/user_file/move_file")
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("wukong move failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func (d *Wukong) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	srcID := srcObj.GetID()
	if srcID == "" {
		return errors.New("missing file id")
	}

	var resp rawResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(map[string]any{
			"file_id":  asIDValue(srcID),
			"new_name": newName,
		}).
		SetResult(&resp).
		Post("/netdisk/user_file/rename_file")
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("wukong rename failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func (d *Wukong) Remove(ctx context.Context, obj model.Obj) error {
	fileID := obj.GetID()
	if fileID == "" {
		return errors.New("missing file id")
	}

	var resp rawResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(map[string]any{
			"file_id_list": []any{asIDValue(fileID)},
		}).
		SetResult(&resp).
		Post("/netdisk/user_file/delete_file")
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("wukong delete failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func (d *Wukong) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	fatherID := dstDir.GetID()
	if fatherID == "" {
		fatherID = d.RootFolderID
	}

	tempFile, err := file.CacheFullInTempFile()
	if err != nil {
		return err
	}
	defer tempFile.Close()
	if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	md5Hex, crc32Hex, err := calcFileMD5AndCRC32(tempFile)
	if err != nil {
		return err
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file.GetName())), ".")
	fileType := detectWukongFileType(file.GetMimetype(), file.GetName())
	size := file.GetSize()
	up(5)

	uploadType := detectUploadType(file.GetMimetype(), file.GetName())
	authToken, err := d.getUploadAuthToken(ctx, uploadType)
	if err != nil {
		return err
	}
	up(10)

	candidates, err := d.getUploadCandidates(ctx, authToken)
	if err != nil {
		return err
	}
	bestHosts := collectCandidateHosts(candidates)
	if len(bestHosts) == 0 {
		return errors.New("wukong upload candidates is empty")
	}
	up(20)

	applyResp, err := d.applyUploadInner(ctx, authToken, uploadType, size, strings.Join(bestHosts, ","))
	if err != nil {
		return err
	}
	if len(applyResp.Result.InnerUploadAddress.UploadNodes) == 0 ||
		len(applyResp.Result.InnerUploadAddress.UploadNodes[0].StoreInfos) == 0 {
		return errors.New("wukong apply upload inner returns empty upload node")
	}
	node := applyResp.Result.InnerUploadAddress.UploadNodes[0]
	store := node.StoreInfos[0]
	up(30)

	if size > multipartChunkSize {
		if err = d.uploadToTOSMultipart(ctx, node.UploadHost, store.StoreURI, store.Auth, getStorageUserID(store.StorageHeader), tempFile, size, up); err != nil {
			return err
		}
	} else {
		if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
			return err
		}
		reader := &driver.ReaderUpdatingProgress{
			Reader: &driver.SimpleReaderWithSize{Reader: tempFile, Size: size},
			UpdateProgress: func(percent float64) {
				up(30 + percent*0.5)
			},
		}
		if err = d.uploadToTOS(ctx, node.UploadHost, store.StoreURI, store.Auth, getStorageUserID(store.StorageHeader), crc32Hex, file.GetName(), reader, size); err != nil {
			return err
		}
	}
	up(85)

	videoVid, err := d.commitUploadInner(ctx, authToken, chooseCommitSpace(uploadType, authToken.SpaceName), node.SessionKey)
	if err != nil {
		return err
	}
	up(92)

	if fileType == 3000 && videoVid == "" {
		return errors.New("wukong video upload missing vid in commit response")
	}
	if err = d.uploadSubmit(ctx, fatherID, file.GetName(), ext, fileType, size, md5Hex, store.StoreURI, videoVid); err != nil {
		return err
	}
	up(100)
	return nil
}

func (d *Wukong) getUploadAuthToken(ctx context.Context, uploadType string) (*uploadAuthTokenResp, error) {
	var resp uploadAuthTokenResp
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"upload_source":   uploadSourceByType(uploadType),
			"type":            uploadType,
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetResult(&resp).
		Get("/toutiao/upload/auth_token/v1/")
	if err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("wukong get upload auth token failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return &resp, nil
}

func (d *Wukong) getUploadCandidates(ctx context.Context, auth *uploadAuthTokenResp) (*getUploadCandidatesResp, error) {
	q := map[string]string{
		"Action":    "GetUploadCandidates",
		"Version":   "2020-11-19",
		"SpaceName": videoSpaceName,
	}
	var resp getUploadCandidatesResp
	if err := d.vodRequest(ctx, http.MethodGet, q, nil, auth, &resp); err != nil {
		return nil, err
	}
	if resp.ResponseMetadata.Error.Code != "" {
		return nil, fmt.Errorf("wukong get upload candidates failed: %s", resp.ResponseMetadata.Error.Message)
	}
	return &resp, nil
}

func (d *Wukong) applyUploadInner(ctx context.Context, auth *uploadAuthTokenResp, uploadType string, fileSize int64, bestHosts string) (*applyUploadInnerResp, error) {
	spaceName := auth.SpaceName
	if uploadType == "video" {
		spaceName = videoSpaceName
	}
	q := map[string]string{
		"Action":          "ApplyUploadInner",
		"Version":         "2020-11-19",
		"SpaceName":       spaceName,
		"FileType":        uploadType,
		"IsInner":         "1",
		"ClientBestHosts": bestHosts,
		"NeedFallback":    "true",
		"FileSize":        strconv.FormatInt(fileSize, 10),
		"s":               randomString(8),
	}
	var resp applyUploadInnerResp
	if err := d.vodRequest(ctx, http.MethodGet, q, nil, auth, &resp); err != nil {
		return nil, err
	}
	if resp.ResponseMetadata.Error.Code != "" {
		return nil, fmt.Errorf("wukong apply upload inner failed: %s", resp.ResponseMetadata.Error.Message)
	}
	return &resp, nil
}

func (d *Wukong) commitUploadInner(ctx context.Context, auth *uploadAuthTokenResp, spaceName, sessionKey string) (string, error) {
	q := map[string]string{
		"Action":    "CommitUploadInner",
		"Version":   "2020-11-19",
		"SpaceName": spaceName,
	}
	body, _ := json.Marshal(map[string]any{
		"SessionKey": sessionKey,
		"Functions":  []any{},
	})
	var resp commitUploadInnerResp
	if err := d.vodRequest(ctx, http.MethodPost, q, body, auth, &resp); err != nil {
		return "", err
	}
	if resp.ResponseMetadata.Error.Code != "" {
		return "", fmt.Errorf("wukong commit upload inner failed: %s", resp.ResponseMetadata.Error.Message)
	}
	if len(resp.Result.Results) > 0 {
		status := resp.Result.Results[0].URIStatus
		if status != 0 && status != minUploadSubmitSuccess {
			return "", fmt.Errorf("wukong commit upload inner failed: uri_status=%d", status)
		}
	}
	return extractVideoVid(&resp), nil
}

func (d *Wukong) uploadSubmit(ctx context.Context, fatherID, fileName, ext string, fileType int, size int64, md5Hex, storeURI, videoVid string) error {
	var resp uploadSubmitResp
	body := map[string]any{
		"base_info": map[string]any{
			"father_id":    asIDValue(fatherID),
			"file_type":    fileType,
			"size":         size,
			"extension":    ext,
			"file_name":    fileName,
			"is_directory": 0,
			"md5":          md5Hex,
			"slice_md5":    md5Hex,
		},
	}
	switch fileType {
	case 3000:
		if videoVid != "" {
			body["video_info"] = map[string]any{"vid": videoVid}
		}
	case 2000:
		body["image_info"] = map[string]any{"uri": storeURI}
	default:
		body["general_info"] = map[string]any{"key": storeURI}
	}
	_, err := d.client.R().
		SetContext(ctx).
		SetQueryParams(map[string]string{
			"aid":             d.Aid,
			"device_platform": "web",
			"language":        d.Language,
		}).
		SetBody(body).
		SetResult(&resp).
		Post("/netdisk/upload_submit/")
	if err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("wukong upload submit failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func extractVideoVid(resp *commitUploadInnerResp) string {
	for _, item := range resp.Result.Results {
		if item.Vid != "" {
			return item.Vid
		}
	}
	if resp.Result.PluginResult != nil {
		if vid := findStringByKey(resp.Result.PluginResult, "Vid"); vid != "" {
			return vid
		}
		if vid := findStringByKey(resp.Result.PluginResult, "vid"); vid != "" {
			return vid
		}
	}
	return ""
}

func findStringByKey(v any, key string) string {
	switch cur := v.(type) {
	case map[string]any:
		if val, ok := cur[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
		for _, child := range cur {
			if s := findStringByKey(child, key); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range cur {
			if s := findStringByKey(child, key); s != "" {
				return s
			}
		}
	}
	return ""
}

func (d *Wukong) uploadToTOS(ctx context.Context, host, storeURI, auth, storageUser, crc32Hex, fileName string, body io.Reader, size int64) error {
	var resp tosUploadResp
	uploadURL := fmt.Sprintf("https://%s/upload/v1/%s", host, storeURI)
	req := base.NewRestyClient().R().
		SetContext(ctx).
		SetHeader("Host", host).
		SetHeader("Referer", webReferer).
		SetHeader("Origin", "https://pan.wkbrowser.com").
		SetHeader("Authorization", auth).
		SetHeader("Content-Type", "application/octet-stream").
		SetHeader("Content-Crc32", crc32Hex).
		SetHeader("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, url.QueryEscape(fileName))).
		SetHeader("Content-Length", strconv.FormatInt(size, 10)).
		SetBody(body).
		SetResult(&resp)
	if storageUser != "" {
		req.SetHeader("X-Storage-U", storageUser)
	}
	_, err := req.Post(uploadURL)
	if err != nil {
		return err
	}
	if resp.Code != minUploadSubmitSuccess {
		return fmt.Errorf("wukong upload to tos failed: code=%d message=%s", resp.Code, resp.Message)
	}
	if resp.Data.Crc32 != "" && !strings.EqualFold(resp.Data.Crc32, crc32Hex) {
		return fmt.Errorf("wukong upload to tos crc32 mismatch: local=%s remote=%s", crc32Hex, resp.Data.Crc32)
	}
	return nil
}

func (d *Wukong) uploadToTOSMultipart(ctx context.Context, host, storeURI, auth, storageUser string, tempFile model.File, size int64, up driver.UpdateProgress) error {
	uploadID, err := d.initMultipartUpload(ctx, host, storeURI, auth, storageUser)
	if err != nil {
		return err
	}

	totalParts := int((size + multipartChunkSize - 1) / multipartChunkSize)
	if totalParts <= 0 {
		return errors.New("invalid multipart parts")
	}
	parts := make([]string, 0, totalParts)
	for i := 0; i < totalParts; i++ {
		partNumber := i + 1
		offset := int64(i) * multipartChunkSize
		partSize := multipartChunkSize
		if remain := size - offset; remain < partSize {
			partSize = remain
		}
		buf := make([]byte, partSize)
		n, readErr := tempFile.ReadAt(buf, offset)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		buf = buf[:n]
		crc32Hex := fmt.Sprintf("%08x", crc32.ChecksumIEEE(buf))
		remoteCRC32, err := d.uploadMultipartPart(ctx, host, storeURI, auth, storageUser, uploadID, partNumber, buf, crc32Hex)
		if err != nil {
			return err
		}
		if remoteCRC32 != "" && !strings.EqualFold(remoteCRC32, crc32Hex) {
			return fmt.Errorf("multipart part crc32 mismatch: part=%d local=%s remote=%s", partNumber, crc32Hex, remoteCRC32)
		}
		parts = append(parts, fmt.Sprintf("%d:%s", partNumber, crc32Hex))
		up(30 + float64(partNumber)/float64(totalParts)*50)
	}

	return d.finishMultipartUpload(ctx, host, storeURI, auth, storageUser, uploadID, strings.Join(parts, ","))
}

func (d *Wukong) initMultipartUpload(ctx context.Context, host, storeURI, auth, storageUser string) (string, error) {
	var resp tosUploadResp
	req := base.NewRestyClient().R().
		SetContext(ctx).
		SetHeader("Host", host).
		SetHeader("Referer", webReferer).
		SetHeader("Origin", "https://pan.wkbrowser.com").
		SetHeader("Authorization", auth).
		SetQueryParams(map[string]string{
			"uploadmode": "part",
			"phase":      "init",
		}).
		SetResult(&resp)
	if storageUser != "" {
		req.SetHeader("X-Storage-U", storageUser)
	}
	uploadURL := fmt.Sprintf("https://%s/upload/v1/%s", host, storeURI)
	_, err := req.Post(uploadURL)
	if err != nil {
		return "", err
	}
	if resp.Code != minUploadSubmitSuccess {
		return "", fmt.Errorf("wukong init multipart upload failed: code=%d message=%s", resp.Code, resp.Message)
	}
	if resp.Data.UploadID == "" {
		return "", errors.New("wukong init multipart upload returns empty uploadid")
	}
	return resp.Data.UploadID, nil
}

func (d *Wukong) uploadMultipartPart(ctx context.Context, host, storeURI, auth, storageUser, uploadID string, partNumber int, data []byte, crc32Hex string) (string, error) {
	var resp tosUploadResp
	req := base.NewRestyClient().R().
		SetContext(ctx).
		SetHeader("Host", host).
		SetHeader("Referer", webReferer).
		SetHeader("Origin", "https://pan.wkbrowser.com").
		SetHeader("Authorization", auth).
		SetHeader("Content-Type", "application/octet-stream").
		SetHeader("Content-Crc32", crc32Hex).
		SetHeader("Content-Length", strconv.Itoa(len(data))).
		SetQueryParams(map[string]string{
			"uploadid":    uploadID,
			"part_number": strconv.Itoa(partNumber),
			"phase":       "transfer",
		}).
		SetBody(data).
		SetResult(&resp)
	if storageUser != "" {
		req.SetHeader("X-Storage-U", storageUser)
	}
	uploadURL := fmt.Sprintf("https://%s/upload/v1/%s", host, storeURI)
	_, err := req.Post(uploadURL)
	if err != nil {
		return "", err
	}
	if resp.Code != minUploadSubmitSuccess {
		return "", fmt.Errorf("wukong multipart transfer failed: code=%d message=%s part=%d", resp.Code, resp.Message, partNumber)
	}
	return resp.Data.Crc32, nil
}

func (d *Wukong) finishMultipartUpload(ctx context.Context, host, storeURI, auth, storageUser, uploadID, body string) error {
	var resp tosUploadResp
	req := base.NewRestyClient().R().
		SetContext(ctx).
		SetHeader("Host", host).
		SetHeader("Referer", webReferer).
		SetHeader("Origin", "https://pan.wkbrowser.com").
		SetHeader("Authorization", auth).
		SetQueryParams(map[string]string{
			"uploadid":   uploadID,
			"phase":      "finish",
			"uploadmode": "part",
		}).
		SetBody(body).
		SetResult(&resp)
	if storageUser != "" {
		req.SetHeader("X-Storage-U", storageUser)
	}
	uploadURL := fmt.Sprintf("https://%s/upload/v1/%s", host, storeURI)
	_, err := req.Post(uploadURL)
	if err != nil {
		return err
	}
	if resp.Code != minUploadSubmitSuccess && resp.Code != 4024 {
		return fmt.Errorf("wukong multipart finish failed: code=%d message=%s", resp.Code, resp.Message)
	}
	return nil
}

func (d *Wukong) vodRequest(ctx context.Context, method string, query map[string]string, body []byte, auth *uploadAuthTokenResp, resp any) error {
	reqURL := vodBaseURL + "/"
	amzDate := time.Now().UTC().Format("20060102T150405Z")
	dateStamp := amzDate[:8]
	headers := map[string]string{
		"x-amz-date":           amzDate,
		"x-amz-security-token": auth.SessionToken,
	}
	if method == http.MethodPost {
		headers["x-amz-content-sha256"] = hashSHA256Bytes(body)
	}
	authorization := buildVodAuthorization(method, "/", query, headers, body, auth, dateStamp)

	req := base.NewRestyClient().R().
		SetContext(ctx).
		SetHeader("Authorization", authorization).
		SetHeader("x-amz-date", amzDate).
		SetHeader("x-amz-security-token", auth.SessionToken).
		SetQueryParams(query).
		SetResult(resp)
	if method == http.MethodPost {
		req.SetHeader("x-amz-content-sha256", headers["x-amz-content-sha256"])
		req.SetHeader("Content-Type", "text/plain;charset=UTF-8")
		req.SetBody(body)
	}
	_, err := req.Execute(method, reqURL)
	return err
}

func buildVodAuthorization(method, canonicalURI string, query map[string]string, headers map[string]string, body []byte, auth *uploadAuthTokenResp, dateStamp string) string {
	canonicalQueryString := getCanonicalQueryStringFromMap(query)
	canonicalHeaders, signedHeaders := getCanonicalHeaders(headers)
	payloadHash := hashSHA256Bytes(body)
	canonicalRequest := method + "\n" + canonicalURI + "\n" + canonicalQueryString + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + payloadHash
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, vodRegion, vodService)
	stringToSign := "AWS4-HMAC-SHA256\n" + headers["x-amz-date"] + "\n" + credentialScope + "\n" + hashSHA256String(canonicalRequest)
	signingKey := getSigningKey(auth.SecretAccessKey, dateStamp, vodRegion, vodService)
	signature := hmacSHA256Hex(signingKey, stringToSign)
	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", auth.AccessKeyID, credentialScope, signedHeaders, signature)
}

func getCanonicalQueryStringFromMap(query map[string]string) string {
	if len(query) == 0 {
		return ""
	}
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, awsURLEncode(k)+"="+awsURLEncode(query[k]))
	}
	return strings.Join(parts, "&")
}

func getCanonicalHeaders(headers map[string]string) (string, string) {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)
	var h strings.Builder
	for _, k := range keys {
		h.WriteString(k)
		h.WriteString(":")
		h.WriteString(strings.TrimSpace(headers[k]))
		h.WriteString("\n")
	}
	return h.String(), strings.Join(keys, ";")
}

func awsURLEncode(s string) string {
	s = url.QueryEscape(s)
	return strings.ReplaceAll(s, "+", "%20")
}

func hashSHA256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashSHA256String(s string) string {
	return hashSHA256Bytes([]byte(s))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}

func hmacSHA256Hex(key []byte, data string) string {
	return hex.EncodeToString(hmacSHA256(key, data))
}

func getSigningKey(secret, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func collectCandidateHosts(resp *getUploadCandidatesResp) []string {
	seen := map[string]struct{}{}
	hosts := make([]string, 0, len(resp.Result.Domains))
	add := func(domain vodDomain) {
		if domain.Name == "" {
			return
		}
		if _, ok := seen[domain.Name]; ok {
			return
		}
		seen[domain.Name] = struct{}{}
		hosts = append(hosts, domain.Name)
	}
	for _, candidate := range resp.Result.Candidates {
		for _, domain := range candidate.Domains {
			add(domain)
		}
	}
	for _, domain := range resp.Result.Domains {
		add(domain)
	}
	return hosts
}

func getStorageUserID(header map[string]any) string {
	if header == nil {
		return ""
	}
	if s, ok := header["USER_ID"].(string); ok {
		return s
	}
	if f, ok := header["USER_ID"].(float64); ok {
		return strconv.FormatInt(int64(f), 10)
	}
	return ""
}

func calcFileMD5AndCRC32(f model.File) (string, string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	md5Hasher := md5.New()
	crc := crc32.NewIEEE()
	_, err := io.Copy(io.MultiWriter(md5Hasher, crc), f)
	if err != nil {
		return "", "", err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(md5Hasher.Sum(nil)), fmt.Sprintf("%08x", crc.Sum32()), nil
}

func detectWukongFileType(mimetype, fileName string) int {
	lowerName := strings.ToLower(fileName)
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		return 2000
	case strings.HasPrefix(mimetype, "video/"), strings.HasSuffix(lowerName, ".flv"), strings.HasSuffix(lowerName, ".mkv"):
		return 3000
	case strings.HasPrefix(mimetype, "audio/"), strings.HasSuffix(lowerName, ".mp3"), strings.HasSuffix(lowerName, ".m4a"), strings.HasSuffix(lowerName, ".wav"):
		return 4000
	case strings.HasSuffix(lowerName, ".zip"), strings.HasSuffix(lowerName, ".rar"), strings.HasSuffix(lowerName, ".7z"), strings.HasSuffix(lowerName, ".tar"), strings.HasSuffix(lowerName, ".gz"), strings.HasSuffix(lowerName, ".tgz"):
		return 6000
	default:
		return 5000
	}
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	if _, err := crand.Read(buf); err == nil {
		for i := range buf {
			buf[i] = letters[int(buf[i])%len(letters)]
		}
		return string(buf)
	}

	now := uint64(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		now = now*6364136223846793005 + 1
		b[i] = letters[int(now%uint64(len(letters)))]
	}
	return string(b)
}

func uploadSourceByType(uploadType string) string {
	switch uploadType {
	case "video":
		return "10150001"
	case "image":
		return "20150001"
	default:
		return "50150001"
	}
}

func detectUploadType(mimetype, fileName string) string {
	lowerName := strings.ToLower(fileName)
	if strings.HasPrefix(mimetype, "video/") || strings.HasPrefix(mimetype, "audio/") ||
		strings.HasSuffix(lowerName, ".flv") || strings.HasSuffix(lowerName, ".mkv") ||
		strings.HasSuffix(lowerName, ".mp3") || strings.HasSuffix(lowerName, ".m4a") || strings.HasSuffix(lowerName, ".wav") {
		return "video"
	}
	if strings.HasPrefix(mimetype, "image/") {
		return "image"
	}
	return "object"
}

func chooseCommitSpace(uploadType, authSpace string) string {
	if uploadType == "video" {
		return videoSpaceName
	}
	return authSpace
}

func asIDValue(id string) any {
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		return n
	}
	return id
}

func parseUnix(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	if ts > 1e12 {
		return time.UnixMilli(ts)
	}
	return time.Unix(ts, 0)
}

func hasMore(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case int64:
		return val != 0
	case string:
		return val == "1" || strings.EqualFold(val, "true")
	default:
		return false
	}
}

func extractURL(data map[string]any) string {
	priority := []string{
		"download_url",
		"main_url",
		"MainUrl",
		"MainHTTPUrl",
		"url",
		"source_url",
		"play_url",
		"backup_url",
		"BackupUrl",
		"BackupHTTPUrl",
	}
	for _, key := range priority {
		if url := findURLByKey(data, key); url != "" {
			return url
		}
	}
	return findAnyHTTPURL(data)
}

func extractDetailMainURL(data map[string]any) string {
	rawList, ok := data["list"]
	if !ok {
		return ""
	}
	list, ok := rawList.([]any)
	if !ok || len(list) == 0 {
		return ""
	}
	first, ok := list[0].(map[string]any)
	if !ok {
		return ""
	}
	generalInfo, ok := first["general_info"].(map[string]any)
	if !ok {
		return ""
	}
	mainURL, ok := generalInfo["main_url"].(string)
	if !ok || !isHTTPURL(mainURL) {
		return ""
	}
	return mainURL
}

func findURLByKey(v any, key string) string {
	switch cur := v.(type) {
	case map[string]any:
		if val, ok := cur[key]; ok {
			if s, ok := val.(string); ok && isHTTPURL(s) {
				return s
			}
			if decoded := tryDecodeJSONAny(val); decoded != nil {
				if s := findURLByKey(decoded, key); s != "" {
					return s
				}
			}
		}
		for _, child := range cur {
			if s := findURLByKey(child, key); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range cur {
			if s := findURLByKey(child, key); s != "" {
				return s
			}
		}
	case string:
		if decoded := tryDecodeJSONString(cur); decoded != nil {
			return findURLByKey(decoded, key)
		}
	}
	return ""
}

func findAnyHTTPURL(v any) string {
	switch cur := v.(type) {
	case string:
		if isHTTPURL(cur) {
			return cur
		}
		if decoded := tryDecodeJSONString(cur); decoded != nil {
			return findAnyHTTPURL(decoded)
		}
	case map[string]any:
		for _, child := range cur {
			if s := findAnyHTTPURL(child); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range cur {
			if s := findAnyHTTPURL(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func isHTTPURL(v string) bool {
	return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
}

func tryDecodeJSONAny(v any) any {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return tryDecodeJSONString(s)
}

func tryDecodeJSONString(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if !(strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

var _ driver.Driver = (*Wukong)(nil)
