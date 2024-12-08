package _115

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	crypto "github.com/gaoyb7/115drive-webdav/115"
	"github.com/orzogc/fake115uploader/cipher"
	"github.com/pkg/errors"
)

// var UserAgent = driver115.UA115Browser
func (d *Pan115) login() error {
	var err error
	opts := []driver115.Option{
		driver115.UA(d.getUA()),
		func(c *driver115.Pan115Client) {
			c.Client.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Conf.TlsInsecureSkipVerify})
		},
	}
	d.client = driver115.New(opts...)
	cr := &driver115.Credential{}
	if d.QRCodeToken != "" {
		s := &driver115.QRCodeSession{
			UID: d.QRCodeToken,
		}
		if cr, err = d.client.QRCodeLoginWithApp(s, driver115.LoginApp(d.QRCodeSource)); err != nil {
			return errors.Wrap(err, "failed to login by qrcode")
		}
		d.Cookie = fmt.Sprintf("UID=%s;CID=%s;SEID=%s;KID=%s", cr.UID, cr.CID, cr.SEID, cr.KID)
		d.QRCodeToken = ""
	} else if d.Cookie != "" {
		if err = cr.FromCookie(d.Cookie); err != nil {
			return errors.Wrap(err, "failed to login by cookies")
		}
		d.client.ImportCredential(cr)
	} else {
		return errors.New("missing cookie or qrcode account")
	}
	return d.client.LoginCheck()
}

func (d *Pan115) getFiles(fileId string) ([]FileObj, error) {
	res := make([]FileObj, 0)
	if d.PageSize <= 0 {
		d.PageSize = driver115.FileListLimit
	}
	files, err := d.client.ListWithLimit(fileId, d.PageSize)
	if err != nil {
		return nil, err
	}
	for _, file := range *files {
		res = append(res, FileObj{file})
	}
	return res, nil
}

func (d *Pan115) getNewFile(fileId string) (*FileObj, error) {
	file, err := d.client.GetFile(fileId)
	if err != nil {
		return nil, err
	}
	return &FileObj{*file}, nil
}

func (d *Pan115) getNewFileByPickCode(pickCode string) (*FileObj, error) {
	result := driver115.GetFileInfoResponse{}
	req := d.client.NewRequest().
		SetQueryParam("pick_code", pickCode).
		ForceContentType("application/json;charset=UTF-8").
		SetResult(&result)
	resp, err := req.Get(driver115.ApiFileInfo)
	if err := driver115.CheckErr(err, &result, resp); err != nil {
		return nil, err
	}
	if len(result.Files) == 0 {
		return nil, errors.New("not get file info")
	}
	fileInfo := result.Files[0]

	f := &FileObj{}
	f.From(fileInfo)
	return f, nil
}

func (d *Pan115) getUA() string {
	return fmt.Sprintf("Mozilla/5.0 115Browser/%s", appVer)
}

func (d *Pan115) DownloadWithUA(pickCode, ua string) (*driver115.DownloadInfo, error) {
	key := crypto.GenerateKey()
	result := driver115.DownloadResp{}
	params, err := utils.Json.Marshal(map[string]string{"pickcode": pickCode})
	if err != nil {
		return nil, err
	}

	data := crypto.Encode(params, key)

	bodyReader := strings.NewReader(url.Values{"data": []string{data}}.Encode())
	reqUrl := fmt.Sprintf("%s?t=%s", driver115.ApiDownloadGetUrl, driver115.Now().String())
	req, _ := http.NewRequest(http.MethodPost, reqUrl, bodyReader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", d.Cookie)
	req.Header.Set("User-Agent", ua)

	resp, err := d.client.Client.GetClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := utils.Json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if err = result.Err(string(body)); err != nil {
		return nil, err
	}

	bytes, err := crypto.Decode(string(result.EncodedData), key)
	if err != nil {
		return nil, err
	}

	downloadInfo := driver115.DownloadData{}
	if err := utils.Json.Unmarshal(bytes, &downloadInfo); err != nil {
		return nil, err
	}

	for _, info := range downloadInfo {
		if info.FileSize < 0 {
			return nil, driver115.ErrDownloadEmpty
		}
		info.Header = resp.Request.Header
		return info, nil
	}
	return nil, driver115.ErrUnexpected
}

func (c *Pan115) GenerateToken(fileID, preID, timeStamp, fileSize, signKey, signVal string) string {
	userID := strconv.FormatInt(c.client.UserID, 10)
	userIDMd5 := md5.Sum([]byte(userID))
	tokenMd5 := md5.Sum([]byte(md5Salt + fileID + fileSize + signKey + signVal + userID + timeStamp + hex.EncodeToString(userIDMd5[:]) + appVer))
	return hex.EncodeToString(tokenMd5[:])
}

func (d *Pan115) rapidUpload(fileSize int64, fileName, dirID, preID, fileID string, stream model.FileStreamer) (*driver115.UploadInitResp, error) {
	var (
		ecdhCipher   *cipher.EcdhCipher
		encrypted    []byte
		decrypted    []byte
		encodedToken string
		err          error
		target       = "U_1_" + dirID
		bodyBytes    []byte
		result       = driver115.UploadInitResp{}
		fileSizeStr  = strconv.FormatInt(fileSize, 10)
	)
	if ecdhCipher, err = cipher.NewEcdhCipher(); err != nil {
		return nil, err
	}

	userID := strconv.FormatInt(d.client.UserID, 10)
	form := url.Values{}
	form.Set("appid", "0")
	form.Set("appversion", appVer)
	form.Set("userid", userID)
	form.Set("filename", fileName)
	form.Set("filesize", fileSizeStr)
	form.Set("fileid", fileID)
	form.Set("target", target)
	form.Set("sig", d.client.GenerateSignature(fileID, target))

	signKey, signVal := "", ""
	for retry := true; retry; {
		t := driver115.NowMilli()

		if encodedToken, err = ecdhCipher.EncodeToken(t.ToInt64()); err != nil {
			return nil, err
		}

		params := map[string]string{
			"k_ec": encodedToken,
		}

		form.Set("t", t.String())
		form.Set("token", d.GenerateToken(fileID, preID, t.String(), fileSizeStr, signKey, signVal))
		if signKey != "" && signVal != "" {
			form.Set("sign_key", signKey)
			form.Set("sign_val", signVal)
		}
		if encrypted, err = ecdhCipher.Encrypt([]byte(form.Encode())); err != nil {
			return nil, err
		}

		req := d.client.NewRequest().
			SetQueryParams(params).
			SetBody(encrypted).
			SetHeaderVerbatim("Content-Type", "application/x-www-form-urlencoded").
			SetDoNotParseResponse(true)
		resp, err := req.Post(driver115.ApiUploadInit)
		if err != nil {
			return nil, err
		}
		data := resp.RawBody()
		defer data.Close()
		if bodyBytes, err = io.ReadAll(data); err != nil {
			return nil, err
		}
		if decrypted, err = ecdhCipher.Decrypt(bodyBytes); err != nil {
			return nil, err
		}
		if err = driver115.CheckErr(json.Unmarshal(decrypted, &result), &result, resp); err != nil {
			return nil, err
		}
		if result.Status == 7 {
			// Update signKey & signVal
			signKey = result.SignKey
			signVal, err = UploadDigestRange(stream, result.SignCheck)
			if err != nil {
				return nil, err
			}
		} else {
			retry = false
		}
		result.SHA1 = fileID
	}

	return &result, nil
}

func UploadDigestRange(stream model.FileStreamer, rangeSpec string) (result string, err error) {
	var start, end int64
	if _, err = fmt.Sscanf(rangeSpec, "%d-%d", &start, &end); err != nil {
		return
	}

	length := end - start + 1
	reader, err := stream.RangeRead(http_range.Range{Start: start, Length: length})
	if err != nil {
		return "", err
	}
	hashStr, err := utils.HashReader(utils.SHA1, reader)
	if err != nil {
		return "", err
	}
	result = strings.ToUpper(hashStr)
	return
}

// UploadByOSS use aliyun sdk to upload
func (c *Pan115) UploadByOSS(params *driver115.UploadOSSParams, r io.Reader, dirID string) (*UploadResult, error) {
	ossToken, err := c.client.GetOSSToken()
	if err != nil {
		return nil, err
	}
	ossClient, err := oss.New(driver115.OSSEndpoint, ossToken.AccessKeyID, ossToken.AccessKeySecret)
	if err != nil {
		return nil, err
	}
	bucket, err := ossClient.Bucket(params.Bucket)
	if err != nil {
		return nil, err
	}

	var bodyBytes []byte
	if err = bucket.PutObject(params.Object, r, append(
		driver115.OssOption(params, ossToken),
		oss.CallbackResult(&bodyBytes),
	)...); err != nil {
		return nil, err
	}

	var uploadResult UploadResult
	if err = json.Unmarshal(bodyBytes, &uploadResult); err != nil {
		return nil, err
	}
	return &uploadResult, uploadResult.Err(string(bodyBytes))
}

// UploadByMultipart upload by mutipart blocks
func (d *Pan115) UploadByMultipart(params *driver115.UploadOSSParams, fileSize int64, stream model.FileStreamer, dirID string, opts ...driver115.UploadMultipartOption) (*UploadResult, error) {
	var (
		chunks    []oss.FileChunk
		parts     []oss.UploadPart
		imur      oss.InitiateMultipartUploadResult
		ossClient *oss.Client
		bucket    *oss.Bucket
		ossToken  *driver115.UploadOSSTokenResp
		bodyBytes []byte
		err       error
	)

	tmpF, err := stream.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}

	options := driver115.DefalutUploadMultipartOptions()
	if len(opts) > 0 {
		for _, f := range opts {
			f(options)
		}
	}
	// oss 启用Sequential必须按顺序上传
	options.ThreadsNum = 1

	if ossToken, err = d.client.GetOSSToken(); err != nil {
		return nil, err
	}

	if ossClient, err = oss.New(driver115.OSSEndpoint, ossToken.AccessKeyID, ossToken.AccessKeySecret, oss.EnableMD5(true), oss.EnableCRC(true)); err != nil {
		return nil, err
	}

	if bucket, err = ossClient.Bucket(params.Bucket); err != nil {
		return nil, err
	}

	// ossToken一小时后就会失效，所以每50分钟重新获取一次
	ticker := time.NewTicker(options.TokenRefreshTime)
	defer ticker.Stop()
	// 设置超时
	timeout := time.NewTimer(options.Timeout)

	if chunks, err = SplitFile(fileSize); err != nil {
		return nil, err
	}

	if imur, err = bucket.InitiateMultipartUpload(params.Object,
		oss.SetHeader(driver115.OssSecurityTokenHeaderName, ossToken.SecurityToken),
		oss.UserAgentHeader(driver115.OSSUserAgent),
		oss.EnableSha1(), oss.Sequential(),
	); err != nil {
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(len(chunks))

	chunksCh := make(chan oss.FileChunk)
	errCh := make(chan error)
	UploadedPartsCh := make(chan oss.UploadPart)
	quit := make(chan struct{})

	// producer
	go chunksProducer(chunksCh, chunks)
	go func() {
		wg.Wait()
		quit <- struct{}{}
	}()

	// consumers
	for i := 0; i < options.ThreadsNum; i++ {
		go func(threadId int) {
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("recovered in %v", r)
				}
			}()
			for chunk := range chunksCh {
				var part oss.UploadPart // 出现错误就继续尝试，共尝试3次
				for retry := 0; retry < 3; retry++ {
					select {
					case <-ticker.C:
						if ossToken, err = d.client.GetOSSToken(); err != nil { // 到时重新获取ossToken
							errCh <- errors.Wrap(err, "刷新token时出现错误")
						}
					default:
					}

					buf := make([]byte, chunk.Size)
					if _, err = tmpF.ReadAt(buf, chunk.Offset); err != nil && !errors.Is(err, io.EOF) {
						continue
					}

					if part, err = bucket.UploadPart(imur, bytes.NewBuffer(buf), chunk.Size, chunk.Number, driver115.OssOption(params, ossToken)...); err == nil {
						break
					}
				}
				if err != nil {
					errCh <- errors.Wrap(err, fmt.Sprintf("上传 %s 的第%d个分片时出现错误：%v", stream.GetName(), chunk.Number, err))
				}
				UploadedPartsCh <- part
			}
		}(i)
	}

	go func() {
		for part := range UploadedPartsCh {
			parts = append(parts, part)
			wg.Done()
		}
	}()
LOOP:
	for {
		select {
		case <-ticker.C:
			// 到时重新获取ossToken
			if ossToken, err = d.client.GetOSSToken(); err != nil {
				return nil, err
			}
		case <-quit:
			break LOOP
		case <-errCh:
			return nil, err
		case <-timeout.C:
			return nil, fmt.Errorf("time out")
		}
	}

	// 不知道啥原因，oss那边分片上传不计算sha1，导致115服务器校验错误
	// params.Callback.Callback = strings.ReplaceAll(params.Callback.Callback, "${sha1}", params.SHA1)
	if _, err := bucket.CompleteMultipartUpload(imur, parts, append(
		driver115.OssOption(params, ossToken),
		oss.CallbackResult(&bodyBytes),
	)...); err != nil {
		return nil, err
	}

	var uploadResult UploadResult
	if err = json.Unmarshal(bodyBytes, &uploadResult); err != nil {
		return nil, err
	}
	return &uploadResult, uploadResult.Err(string(bodyBytes))
}

func chunksProducer(ch chan oss.FileChunk, chunks []oss.FileChunk) {
	for _, chunk := range chunks {
		ch <- chunk
	}
}

func SplitFile(fileSize int64) (chunks []oss.FileChunk, err error) {
	for i := int64(1); i < 10; i++ {
		if fileSize < i*utils.GB { // 文件大小小于iGB时分为i*1000片
			if chunks, err = SplitFileByPartNum(fileSize, int(i*1000)); err != nil {
				return
			}
			break
		}
	}
	if fileSize > 9*utils.GB { // 文件大小大于9GB时分为10000片
		if chunks, err = SplitFileByPartNum(fileSize, 10000); err != nil {
			return
		}
	}
	// 单个分片大小不能小于100KB
	if chunks[0].Size < 100*utils.KB {
		if chunks, err = SplitFileByPartSize(fileSize, 100*utils.KB); err != nil {
			return
		}
	}
	return
}

// SplitFileByPartNum splits big file into parts by the num of parts.
// Split the file with specified parts count, returns the split result when error is nil.
func SplitFileByPartNum(fileSize int64, chunkNum int) ([]oss.FileChunk, error) {
	if chunkNum <= 0 || chunkNum > 10000 {
		return nil, errors.New("chunkNum invalid")
	}

	if int64(chunkNum) > fileSize {
		return nil, errors.New("oss: chunkNum invalid")
	}

	var chunks []oss.FileChunk
	chunk := oss.FileChunk{}
	chunkN := (int64)(chunkNum)
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * (fileSize / chunkN)
		if i == chunkN-1 {
			chunk.Size = fileSize/chunkN + fileSize%chunkN
		} else {
			chunk.Size = fileSize / chunkN
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// SplitFileByPartSize splits big file into parts by the size of parts.
// Splits the file by the part size. Returns the FileChunk when error is nil.
func SplitFileByPartSize(fileSize int64, chunkSize int64) ([]oss.FileChunk, error) {
	if chunkSize <= 0 {
		return nil, errors.New("chunkSize invalid")
	}

	chunkN := fileSize / chunkSize
	if chunkN >= 10000 {
		return nil, errors.New("Too many parts, please increase part size")
	}

	var chunks []oss.FileChunk
	chunk := oss.FileChunk{}
	for i := int64(0); i < chunkN; i++ {
		chunk.Number = int(i + 1)
		chunk.Offset = i * chunkSize
		chunk.Size = chunkSize
		chunks = append(chunks, chunk)
	}

	if fileSize%chunkSize > 0 {
		chunk.Number = len(chunks) + 1
		chunk.Offset = int64(len(chunks)) * chunkSize
		chunk.Size = fileSize % chunkSize
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
