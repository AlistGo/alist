package yunpan360

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	stdpath "path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	aliststream "github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
)

const (
	yunpanUploadChunkSize = int64(512 * 1024)
	yunpanUploadBoundary  = "WebKitFormBoundaryQ5OJVvzZwEkg4ttY"
	yunpanUploadVersion   = "1.0.1"
	yunpanUploadDevType   = "ecs_openapi"
	yunpanUploadDevName   = "EYUN_WEB_UPLOAD"
)

type openUploadPlan struct {
	DirPath    string
	TargetPath string
	FileName   string
	Size       int64
	FileHash   string
	FileSHA1   string
	FileSum    string
	CreatedAt  int64
	DeviceID   string
	Chunks     []openUploadChunk
}

type openUploadChunk struct {
	Index  int
	Offset int64
	Size   int64
	Hash   string
}

type openUploadDetectResp struct {
	BaseResp
	Data struct {
		Exists  []openUploadDuplicate `json:"exists"`
		IsSlice int                   `json:"is_slice"`
	} `json:"data"`
}

type openUploadDuplicate struct {
	FullName string `json:"fullName"`
}

type openUploadAddressResp struct {
	BaseResp
	Data struct {
		HTTP       string      `json:"http"`
		Addr1      string      `json:"addr_1"`
		Addr2      string      `json:"addr_2"`
		Backup     string      `json:"backup"`
		TK         string      `json:"tk"`
		GroupSize  string      `json:"group_size"`
		AutoCommit interface{} `json:"autoCommit"`
		IsHTTPS    interface{} `json:"is_https"`
	} `json:"data"`
}

type openUploadRequestResp struct {
	BaseResp
	Data struct {
		Tid       string                   `json:"tid"`
		BlockInfo []map[string]interface{} `json:"block_info"`
	} `json:"data"`
}

type openUploadFinalizeResp struct {
	BaseResp
	Data map[string]interface{} `json:"data"`
}

type uploadEnvelope struct {
	Errno  *int            `json:"errno"`
	Errmsg string          `json:"errmsg"`
	Data   json.RawMessage `json:"data"`
}

func (d *Yunpan360) putOpenFile(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	auth, err := d.getOpenAuth(ctx)
	if err != nil {
		return nil, err
	}

	dirPath := dstDir.GetPath()
	if dirPath == "" {
		dirPath = d.RootFolderPath
	}
	dirPath = ensureDirAPIPath(dirPath)
	targetPath := joinRemotePath(dirPath, file.GetName())

	cached, err := d.cacheUploadSource(ctx, file, progressRange(up, 0, 5))
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = cached.Seek(0, io.SeekStart)
	}()

	plan, err := d.buildUploadPlan(ctx, cached, targetPath, file.GetSize(), file.ModTime(), progressRange(up, 5, 10))
	if err != nil {
		return nil, err
	}

	detectResp, err := d.openDetectUpload(ctx, auth, plan)
	if err != nil {
		return nil, err
	}
	if detectResp.Data.IsSlice == 0 {
		detectResp.Data.IsSlice = 1
	}

	addrResp, err := d.openGetUploadAddress(ctx, auth, plan)
	if err != nil {
		return nil, err
	}

	finalResp := &openUploadFinalizeResp{BaseResp: BaseResp{Errno: 0}, Data: map[string]interface{}{}}
	if strings.TrimSpace(addrResp.Data.HTTP) == "" {
		finalResp.Data = map[string]interface{}{"autoCommit": true}
		if tk := strings.TrimSpace(addrResp.Data.TK); tk != "" {
			finalResp.Data["tk"] = tk
			finalResp.Data["autoCommit"] = false
		}
	} else {
		reqResp, err := d.openRequestUpload(ctx, auth, plan, addrResp)
		if err != nil {
			return nil, err
		}
		if err := d.openUploadBlocks(ctx, auth, cached, plan, addrResp, reqResp, up); err != nil {
			return nil, err
		}
		finalResp, err = d.openCommitUpload(ctx, auth, plan, addrResp, reqResp)
		if err != nil {
			return nil, err
		}
	}

	if err := d.openFinalizeUpload(ctx, auth, finalResp); err != nil {
		return nil, err
	}
	if up != nil {
		up(100)
	}

	obj, err := d.findUploadedObject(ctx, targetPath)
	if err == nil {
		return obj, nil
	}
	if !errors.Is(err, errs.ObjectNotFound) {
		return nil, err
	}

	return &model.Object{
		Path:     normalizeRemotePath(targetPath),
		Name:     file.GetName(),
		Size:     file.GetSize(),
		Modified: time.Now(),
		Ctime:    time.Now(),
		HashInfo: utils.NewHashInfo(utils.SHA1, firstNonEmpty(plan.FileSHA1, plan.FileHash)),
	}, nil
}

func (d *Yunpan360) cacheUploadSource(ctx context.Context, file model.FileStreamer, up driver.UpdateProgress) (model.File, error) {
	if cached := file.GetFile(); cached != nil {
		_, _ = cached.Seek(0, io.SeekStart)
		if up != nil {
			up(100)
		}
		return cached, nil
	}
	if up == nil {
		return file.CacheFullInTempFile()
	}
	return aliststream.CacheFullInTempFileAndUpdateProgress(file, up)
}

func (d *Yunpan360) buildUploadPlan(ctx context.Context, cached model.File, targetPath string, size int64, modTime time.Time, up driver.UpdateProgress) (*openUploadPlan, error) {
	if _, err := cached.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	createdAt := time.Now().Unix()
	if !modTime.IsZero() {
		createdAt = modTime.Unix()
	}
	plan := &openUploadPlan{
		DirPath:    ensureDirAPIPath(stdpath.Dir(targetPath)),
		TargetPath: normalizeRemotePath(targetPath),
		FileName:   stdpath.Base(targetPath),
		Size:       size,
		CreatedAt:  createdAt,
		DeviceID:   sha1HexString("node-sdk-" + runtime.Version()),
	}

	if plan.DirPath == "./" || plan.DirPath == "." {
		plan.DirPath = "/"
	}

	totalChunks := 0
	if size > 0 {
		totalChunks = int((size + yunpanUploadChunkSize - 1) / yunpanUploadChunkSize)
	}
	chunks := make([]openUploadChunk, 0, totalChunks)
	var hashConcat strings.Builder
	var hashed int64

	for idx := 0; idx < totalChunks; idx++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		offset := int64(idx) * yunpanUploadChunkSize
		chunkSize := yunpanUploadChunkSize
		if remain := size - offset; remain < chunkSize {
			chunkSize = remain
		}

		chunkHash, err := sha1HexReader(io.NewSectionReader(cached, offset, chunkSize))
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, openUploadChunk{
			Index:  idx + 1,
			Offset: offset,
			Size:   chunkSize,
			Hash:   chunkHash,
		})
		hashConcat.WriteString(chunkHash)
		hashed += chunkSize
		reportByteProgress(up, hashed, size)
	}

	plan.Chunks = chunks
	plan.FileHash = sha1HexString(hashConcat.String())
	if _, err := cached.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	sha1Hasher := sha1.New()
	md5Hasher := md5.New()
	if _, err := io.Copy(io.MultiWriter(sha1Hasher, md5Hasher), cached); err != nil {
		return nil, err
	}
	plan.FileSHA1 = hex.EncodeToString(sha1Hasher.Sum(nil))
	plan.FileSum = hex.EncodeToString(md5Hasher.Sum(nil))
	if size == 0 && up != nil {
		up(100)
	}
	return plan, nil
}

func (d *Yunpan360) openDetectUpload(ctx context.Context, auth *OpenAuthInfo, plan *openUploadPlan) (*openUploadDetectResp, error) {
	payload, err := json.Marshal([]map[string]interface{}{
		{"fname": plan.FileName, "fsize": plan.Size},
	})
	if err != nil {
		return nil, err
	}

	signParams := map[string]string{
		"data": string(payload),
		"path": plan.DirPath,
	}
	body, contentType, err := createMultipartForm("", map[string]string{
		"qid":  auth.Qid,
		"data": string(payload),
		"path": plan.DirPath,
		"sign": openSign(auth.AccessToken, auth.Qid, "Sync.detectFileExists", signParams),
	}, nil)
	if err != nil {
		return nil, err
	}

	var resp openUploadDetectResp
	err = d.uploadRequest(ctx, http.MethodPost, buildJSQueryURL(openAPIURL(d.EcsEnv), "Sync.detectFileExists", nil), auth.AccessToken, contentType, body, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openGetUploadAddress(ctx context.Context, auth *OpenAuthInfo, plan *openUploadPlan) (*openUploadAddressResp, error) {
	query := d.uploadCookieParams(auth, plan, "")
	signParams := map[string]string{
		"access_token": auth.AccessToken,
		"fhash":        plan.FileHash,
		"fname":        plan.TargetPath,
		"fsize":        strconv.FormatInt(plan.Size, 10),
	}
	query["sign"] = openSign(auth.AccessToken, auth.Qid, "Sync.getUploadFileAddr", signParams)

	var resp openUploadAddressResp
	err := d.uploadRequest(ctx, http.MethodGet, buildJSQueryURL(openAPIURL(d.EcsEnv), "Sync.getUploadFileAddr", query), auth.AccessToken, "", nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openRequestUpload(ctx context.Context, auth *OpenAuthInfo, plan *openUploadPlan, addrResp *openUploadAddressResp) (*openUploadRequestResp, error) {
	chunkInfos := make([]map[string]interface{}, 0, len(plan.Chunks))
	for _, chunk := range plan.Chunks {
		chunkInfos = append(chunkInfos, map[string]interface{}{
			"bhash":   chunk.Hash,
			"bidx":    chunk.Index,
			"boffset": chunk.Offset,
			"bsize":   chunk.Size,
		})
	}
	payload, err := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"block_info": chunkInfos,
		},
	})
	if err != nil {
		return nil, err
	}

	body, contentType, err := createMultipartForm(
		yunpanUploadBoundary,
		d.uploadCookieParams(auth, plan, strings.TrimSpace(addrResp.Data.TK)),
		&multipartFile{
			FieldName:   "file",
			FileName:    "file.dat",
			ContentType: "application/octet-stream",
			Content:     payload,
		},
	)
	if err != nil {
		return nil, err
	}

	url := buildJSQueryURL(d.uploadBaseURL(addrResp), "Upload.request4Web", d.uploadDataParams(auth, plan))
	url = appendHostQuery(url, addrResp.Data.HTTP)

	var resp openUploadRequestResp
	err = d.uploadRequest(ctx, http.MethodPost, url, auth.AccessToken, contentType, body, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openUploadBlocks(ctx context.Context, auth *OpenAuthInfo, cached model.File, plan *openUploadPlan, addrResp *openUploadAddressResp, reqResp *openUploadRequestResp, up driver.UpdateProgress) error {
	url := buildJSQueryURL(d.uploadBaseURL(addrResp), "Upload.block4Web", d.uploadDataParams(auth, plan))
	url = appendHostQuery(url, addrResp.Data.HTTP)

	var uploaded int64
	for _, chunk := range plan.Chunks {
		info := reqResp.blockInfoForChunk(chunk.Index)
		if info.found() > 0 {
			uploaded += chunk.Size
			reportUploadProgress(up, uploaded, plan.Size)
			continue
		}

		chunkBytes := make([]byte, chunk.Size)
		if _, err := cached.ReadAt(chunkBytes, chunk.Offset); err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		fields := map[string]string{
			"bhash":    chunk.Hash,
			"bidx":     strconv.Itoa(chunk.Index),
			"boffset":  strconv.FormatInt(chunk.Offset, 10),
			"bsize":    strconv.FormatInt(chunk.Size, 10),
			"filename": plan.TargetPath,
			"filesize": strconv.FormatInt(plan.Size, 10),
			"q":        info.stringValue("q"),
			"t":        info.stringValue("t"),
			"token":    auth.Token,
			"tid":      info.stringValue("tid"),
		}
		for key, value := range info.extraFields() {
			fields[key] = value
		}

		body, contentType, err := createMultipartForm(
			yunpanUploadBoundary,
			fields,
			&multipartFile{
				FieldName:   "file",
				FileName:    "file.dat",
				ContentType: "application/octet-stream",
				Content:     chunkBytes,
			},
		)
		if err != nil {
			return err
		}

		chunkStart := uploaded
		chunkSize := chunk.Size
		err = d.uploadRequestWithProgress(ctx, http.MethodPost, url, auth.AccessToken, contentType, body, func(p float64) {
			done := chunkStart + int64(float64(chunkSize)*(p/100.0))
			reportUploadProgress(up, done, plan.Size)
		}, nil)
		if err != nil {
			return err
		}

		uploaded += chunk.Size
		reportUploadProgress(up, uploaded, plan.Size)
	}
	return nil
}

func (d *Yunpan360) openCommitUpload(ctx context.Context, auth *OpenAuthInfo, plan *openUploadPlan, addrResp *openUploadAddressResp, reqResp *openUploadRequestResp) (*openUploadFinalizeResp, error) {
	body, contentType, err := createMultipartForm("", map[string]string{
		"q":     "",
		"t":     "",
		"token": auth.Token,
		"tid":   strings.TrimSpace(reqResp.Data.Tid),
	}, nil)
	if err != nil {
		return nil, err
	}

	url := buildJSQueryURL(d.uploadBaseURL(addrResp), "Upload.commit4Web", d.uploadDataParams(auth, plan))
	url = appendHostQuery(url, addrResp.Data.HTTP)

	var resp openUploadFinalizeResp
	err = d.uploadRequest(ctx, http.MethodPost, url, auth.AccessToken, contentType, body, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openFinalizeUpload(ctx context.Context, auth *OpenAuthInfo, resp *openUploadFinalizeResp) error {
	if resp == nil || resp.autoCommit() {
		return nil
	}
	tk := strings.TrimSpace(resp.stringValue("tk"))
	if tk == "" {
		return errors.New("upload tk is empty")
	}

	signParams := map[string]string{
		"tk": tk,
	}
	body, contentType, err := createMultipartForm("", map[string]string{
		"qid":  auth.Qid,
		"tk":   tk,
		"sign": openSign(auth.AccessToken, auth.Qid, "Sync.addFileToApi", signParams),
	}, nil)
	if err != nil {
		return err
	}

	return d.uploadRequest(ctx, http.MethodPost, buildJSQueryURL(openAPIURL(d.EcsEnv), "Sync.addFileToApi", nil), auth.AccessToken, contentType, body, nil)
}

func (d *Yunpan360) findUploadedObject(ctx context.Context, targetPath string) (model.Obj, error) {
	targetPath = normalizeRemotePath(targetPath)
	parentPath := normalizeRemotePath(stdpath.Dir(targetPath))
	if parentPath == "." || parentPath == "" {
		parentPath = "/"
	}
	targetName := stdpath.Base(targetPath)

	for page := 0; ; page++ {
		resp, err := d.listPage(ctx, parentPath, page, d.PageSize)
		if err != nil {
			return nil, err
		}
		pageObjs := resp.Objects(parentPath)
		for _, obj := range pageObjs {
			if normalizeRemotePath(obj.GetPath()) == targetPath || obj.GetName() == targetName {
				return obj, nil
			}
		}
		if len(pageObjs) == 0 || len(pageObjs) < d.PageSize {
			break
		}
	}
	return nil, errs.ObjectNotFound
}

func (d *Yunpan360) uploadCookieParams(auth *OpenAuthInfo, plan *openUploadPlan, uploadTK string) map[string]string {
	params := map[string]string{
		"owner_qid": auth.Qid,
		"fname":     plan.TargetPath,
		"fsize":     strconv.FormatInt(plan.Size, 10),
		"fctime":    strconv.FormatInt(plan.CreatedAt, 10),
		"fmtime":    strconv.FormatInt(plan.CreatedAt, 10),
		"fhash":     plan.FileHash,
		"qid":       auth.Qid,
		"fattr":     "0",
		"token":     auth.Token,
		"devtype":   yunpanUploadDevType,
	}
	if uploadTK != "" {
		params["tk"] = uploadTK
	}
	return params
}

func (d *Yunpan360) uploadDataParams(auth *OpenAuthInfo, plan *openUploadPlan) map[string]string {
	return map[string]string{
		"owner_qid": auth.Qid,
		"qid":       auth.Qid,
		"devtype":   yunpanUploadDevType,
		"devid":     plan.DeviceID,
		"v":         yunpanUploadVersion,
		"ofmt":      "json",
		"devname":   yunpanUploadDevName,
		"rtick":     strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
}

func (d *Yunpan360) uploadBaseURL(addrResp *openUploadAddressResp) string {
	host := ""
	isHTTPS := false
	if addrResp != nil {
		host = strings.TrimSpace(addrResp.Data.HTTP)
		isHTTPS = parseOpenDir(addrResp.Data.IsHTTPS)
	}
	scheme := "http"
	if isHTTPS {
		scheme = "https"
	}
	if host == "" {
		return openAPIURL(d.EcsEnv)
	}
	return fmt.Sprintf("%s://%s/intf.php", scheme, host)
}

func (d *Yunpan360) uploadRequest(ctx context.Context, method, reqURL, accessToken, contentType string, body []byte, out interface{}) error {
	return d.uploadRequestWithProgress(ctx, method, reqURL, accessToken, contentType, body, nil, out)
}

func (d *Yunpan360) uploadRequestWithProgress(ctx context.Context, method, reqURL, accessToken, contentType string, body []byte, progress driver.UpdateProgress, out interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, time.Duration(attempt)*500*time.Millisecond); err != nil {
				return err
			}
		}
		err := d.doUploadRequest(ctx, method, reqURL, accessToken, contentType, body, progress, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return lastErr
}

func (d *Yunpan360) doUploadRequest(ctx context.Context, method, reqURL, accessToken, contentType string, body []byte, progress driver.UpdateProgress, out interface{}) error {
	var bodyReader io.ReadCloser
	if body != nil {
		reader := &driver.SimpleReaderWithSize{
			Reader: bytes.NewReader(body),
			Size:   int64(len(body)),
		}
		if progress != nil {
			bodyReader = driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
				Reader:         reader,
				UpdateProgress: progress,
			})
		} else {
			bodyReader = driver.NewLimitedUploadStream(ctx, reader)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		if bodyReader != nil {
			_ = bodyReader.Close()
		}
		return err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	req.Header.Set("Accept", "application/json")
	if accessToken != "" {
		req.Header.Set("Access-Token", accessToken)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := base.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("yunpan upload request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return decodeUploadResp(respBody, out)
}

func decodeUploadResp(body []byte, out interface{}) error {
	var env uploadEnvelope
	if err := utils.Json.Unmarshal(body, &env); err != nil {
		return err
	}
	if env.Errno != nil && *env.Errno != 0 {
		if env.Errmsg == "" {
			return fmt.Errorf("yunpan upload request failed: errno=%d", *env.Errno)
		}
		return errors.New(env.Errmsg)
	}
	if env.Errno == nil && strings.TrimSpace(env.Errmsg) != "" && len(env.Data) > 0 && string(env.Data) == "[]" {
		return errors.New(env.Errmsg)
	}
	if out == nil {
		return nil
	}
	if err := utils.Json.Unmarshal(body, out); err != nil {
		if strings.TrimSpace(env.Errmsg) != "" {
			return errors.New(env.Errmsg)
		}
		return err
	}
	return nil
}

type multipartFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Content     []byte
}

func createMultipartForm(boundary string, fields map[string]string, file *multipartFile) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if boundary != "" {
		if err := writer.SetBoundary(boundary); err != nil {
			return nil, "", err
		}
	}

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}

	if file != nil {
		partHeader := make(textproto.MIMEHeader)
		partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, file.FieldName, file.FileName))
		partHeader.Set("Content-Type", firstNonEmpty(file.ContentType, "application/octet-stream"))
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(file.Content); err != nil {
			return nil, "", err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func buildJSQueryURL(baseURL, method string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString(baseURL)
	if strings.Contains(baseURL, "?") {
		builder.WriteByte('&')
	} else {
		builder.WriteByte('?')
	}
	builder.WriteString("method=")
	builder.WriteString(jsQueryEscape(method))
	for _, key := range keys {
		value := params[key]
		if value == "" {
			continue
		}
		builder.WriteByte('&')
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(jsQueryEscape(value))
	}
	return builder.String()
}

func buildQueryURL(baseURL string, params map[string]string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	u.RawQuery = encodeSortedQuery(params)
	return u.String()
}

func encodeSortedQuery(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	q := make(url.Values, len(params))
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		q.Set(key, params[key])
	}
	return q.Encode()
}

func appendHostQuery(rawURL, host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return rawURL
	}
	return rawURL + "&host=" + jsQueryEscape(host)
}

func jsQueryEscape(raw string) string {
	replacer := strings.NewReplacer(
		"+", "%20",
		"%21", "!",
		"%27", "'",
		"%28", "(",
		"%29", ")",
		"%2A", "*",
		"%7E", "~",
	)
	return replacer.Replace(url.QueryEscape(raw))
}

func sha1HexReader(r io.Reader) (string, error) {
	h := sha1.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha1HexString(raw string) string {
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func progressRange(up driver.UpdateProgress, start, end float64) driver.UpdateProgress {
	if up == nil {
		return nil
	}
	return model.UpdateProgressWithRange(up, start, end)
}

func reportByteProgress(up driver.UpdateProgress, done, total int64) {
	if up == nil {
		return
	}
	if total <= 0 {
		up(100)
		return
	}
	up(float64(done) / float64(total) * 100)
}

func reportUploadProgress(up driver.UpdateProgress, done, total int64) {
	if up == nil {
		return
	}
	if total <= 0 {
		up(100)
		return
	}
	if done > total {
		done = total
	}
	up(10 + float64(done)/float64(total)*90)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type blockInfoMap map[string]interface{}

func (r *openUploadRequestResp) blockInfoForChunk(index int) blockInfoMap {
	if index <= 0 || index > len(r.Data.BlockInfo) {
		return blockInfoMap{}
	}
	return blockInfoMap(r.Data.BlockInfo[index-1])
}

func (m blockInfoMap) stringValue(key string) string {
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprint(v)
	}
}

func (m blockInfoMap) found() int64 {
	raw := strings.TrimSpace(m.stringValue("found"))
	if raw == "" {
		return 0
	}
	value, _ := strconv.ParseInt(raw, 10, 64)
	return value
}

func (m blockInfoMap) extraFields() map[string]string {
	extras := make(map[string]string)
	for key := range m {
		switch key {
		case "bhash", "bidx", "boffset", "bsize", "filename", "filesize", "q", "t", "token", "tid", "found", "url":
			continue
		}
		value := strings.TrimSpace(m.stringValue(key))
		if value != "" {
			extras[key] = value
		}
	}
	return extras
}

func (r *openUploadFinalizeResp) autoCommit() bool {
	raw, ok := r.Data["autoCommit"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case float64:
		return int64(v) == 1
	case int:
		return v == 1
	case int64:
		return v == 1
	default:
		return false
	}
}

func (r *openUploadFinalizeResp) stringValue(key string) string {
	if r == nil || r.Data == nil {
		return ""
	}
	value, ok := r.Data[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}
