package guangyapan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	accountBaseURL = "https://account.guangyapan.com"
	apiBaseURL     = "https://api.guangyapan.com"
	defaultClient  = "aMe-8VSlkrbQXpUR"
)

type GuangYaPan struct {
	model.Storage
	Addition

	accountClient *resty.Client
	apiClient     *resty.Client

	resolvedRootFolderID string
	rootFolderResolved   bool
}

func (d *GuangYaPan) Config() driver.Config {
	return config
}

func (d *GuangYaPan) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *GuangYaPan) Init(ctx context.Context) error {
	d.ClientID = strings.TrimSpace(d.ClientID)
	if d.ClientID == "" {
		d.ClientID = defaultClient
	}
	d.DeviceID = normalizeDeviceID(d.DeviceID)
	if d.DeviceID == "" {
		d.DeviceID = randomDeviceID()
	}
	if d.PageSize <= 0 {
		d.PageSize = 100
	}
	if d.OrderBy < 0 {
		d.OrderBy = 3
	}
	if d.SortType != 0 && d.SortType != 1 {
		d.SortType = 1
	}

	d.RootPath = strings.TrimSpace(d.RootPath)
	d.AccessToken = strings.TrimSpace(d.AccessToken)
	d.RefreshToken = strings.TrimSpace(d.RefreshToken)
	d.PhoneNumber = strings.TrimSpace(d.PhoneNumber)
	d.VerifyCode = strings.TrimSpace(d.VerifyCode)
	d.CaptchaToken = strings.TrimSpace(d.CaptchaToken)
	d.VerificationID = strings.TrimSpace(d.VerificationID)
	d.resolvedRootFolderID = ""
	d.rootFolderResolved = false

	d.accountClient = base.NewRestyClient().
		SetBaseURL(accountBaseURL).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Content-Type", "application/json").
		SetHeader("X-Device-Model", "chrome%2F147.0.0.0").
		SetHeader("X-Device-Name", "PC-Chrome").
		SetHeader("X-Device-Sign", "wdi10."+d.DeviceID+"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx").
		SetHeader("X-Net-Work-Type", "NONE").
		SetHeader("X-OS-Version", "MacIntel").
		SetHeader("X-Platform-Version", "1").
		SetHeader("X-Protocol-Version", "301").
		SetHeader("X-Provider-Name", "NONE").
		SetHeader("X-SDK-Version", "9.0.2").
		SetHeader("X-Client-Id", d.ClientID).
		SetHeader("X-Client-Version", "0.0.1").
		SetHeader("X-Device-Id", d.DeviceID)
	if d.CaptchaToken != "" {
		d.accountClient.SetHeader("X-Captcha-Token", d.CaptchaToken)
	}

	d.apiClient = base.NewRestyClient().
		SetBaseURL(apiBaseURL).
		SetHeader("Accept", "application/json, text/plain, */*").
		SetHeader("Content-Type", "application/json").
		SetHeader("Did", d.DeviceID).
		SetHeader("Dt", "4")

	// Priority: access_token -> refresh_token -> sms login.
	if d.AccessToken != "" {
		if err := d.validateToken(ctx); err == nil {
			return d.prepareRootFolder(ctx)
		}
		d.AccessToken = ""
	}
	if d.RefreshToken != "" {
		if err := d.refreshToken(ctx); err == nil {
			if err2 := d.validateToken(ctx); err2 == nil {
				return d.prepareRootFolder(ctx)
			}
		}
	}
	// Two-stage SMS flow:
	// 1) phone only + send_code=true: send code and cache verification_id (do not fail init).
	// 2) phone + verify_code: complete login and save tokens.
	if d.PhoneNumber != "" {
		if d.canSMSLogin() {
			if err := d.loginBySMSCode(ctx); err != nil {
				return err
			}
			if err := d.validateToken(ctx); err != nil {
				return err
			}
			return d.prepareRootFolder(ctx)
		}
		if d.SendCode {
			d.setTempStatus("SMS sending in progress...")
			if err := d.prepareSMSCode(ctx); err != nil {
				d.setTempStatus(fmt.Sprintf("SMS send failed: %v. Please check captcha/meta and set send_code=true to retry.", err))
				log.Warnf("guangyapan: prepare sms code failed: %v", err)
			} else {
				d.setTempStatus("SMS sent successfully. Please fill verify_code and save to complete login.")
			}
		}
		return nil
	}
	return errors.New("login failed: provide a valid access_token, or refresh_token, or phone_number + verify_code + captcha_token")
}

func (d *GuangYaPan) Drop(ctx context.Context) error {
	return nil
}

func (d *GuangYaPan) GetRoot(ctx context.Context) (model.Obj, error) {
	rootID, err := d.getRootFolderID(ctx)
	if err != nil {
		return nil, err
	}
	return &model.Object{
		ID:       rootID,
		Path:     "/",
		Name:     "root",
		Size:     0,
		Modified: d.Modified,
		IsFolder: true,
	}, nil
}

func (d *GuangYaPan) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if err := d.ensureAccessToken(ctx); err != nil {
		return nil, err
	}

	parentID := dir.GetID()

	res := make([]model.Obj, 0, d.PageSize)
	for page := 0; ; page++ {
		var resp listResp
		body := map[string]any{
			"parentId":  parentID,
			"page":      page,
			"pageSize":  d.PageSize,
			"orderBy":   d.OrderBy,
			"sortType":  d.SortType,
			"fileTypes": []int{},
		}
		if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/get_file_list", body, &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Data.List {
			res = append(res, &model.Object{
				ID:       item.FileID,
				Path:     parentID,
				Name:     item.FileName,
				Size:     item.FileSize,
				Modified: unixOrZero(item.UTime),
				Ctime:    unixOrZero(item.CTime),
				IsFolder: item.ResType == 2,
			})
		}
		if len(resp.Data.List) < d.PageSize {
			break
		}
		if resp.Data.Total > 0 && len(res) >= resp.Data.Total {
			break
		}
	}
	return res, nil
}

func (d *GuangYaPan) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if file.IsDir() {
		return nil, errs.NotFile
	}
	if err := d.ensureAccessToken(ctx); err != nil {
		return nil, err
	}

	var resp downloadResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/get_res_download_url", map[string]any{
		"fileId": file.GetID(),
	}, &resp); err != nil {
		return nil, err
	}

	url := strings.TrimSpace(resp.Data.SignedURL)
	if url == "" {
		url = strings.TrimSpace(resp.Data.DownloadURL)
	}
	if url == "" {
		return nil, errors.New("empty download url")
	}
	return &model.Link{URL: url}, nil
}

func (d *GuangYaPan) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}

	name := strings.TrimSpace(dirName)
	if name == "" {
		return errors.New("dir name is empty")
	}

	parentID := parentDir.GetID()

	var out createDirResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/create_dir", map[string]any{
		"parentId": parentID,
		"dirName":  name,
	}, &out); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(out.Msg), "success") {
		return fmt.Errorf("make dir failed: %s", strings.TrimSpace(out.Msg))
	}
	return nil
}

func (d *GuangYaPan) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}

	fileID := strings.TrimSpace(srcObj.GetID())
	if fileID == "" {
		return errors.New("file id is empty")
	}
	name := strings.TrimSpace(newName)
	if name == "" {
		return errors.New("new name is empty")
	}

	var out commonResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/rename", map[string]any{
		"fileId":  fileID,
		"newName": name,
	}, &out); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(out.Msg), "success") {
		return fmt.Errorf("rename failed: %s", strings.TrimSpace(out.Msg))
	}
	return nil
}

func (d *GuangYaPan) Remove(ctx context.Context, obj model.Obj) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}

	fileID := strings.TrimSpace(obj.GetID())
	if fileID == "" {
		return errors.New("file id is empty")
	}

	var del deleteResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/delete_file", map[string]any{
		"fileIds": []string{fileID},
	}, &del); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(del.Msg), "success") {
		return fmt.Errorf("delete failed: %s", strings.TrimSpace(del.Msg))
	}

	taskID := strings.TrimSpace(del.Data.TaskID)
	if taskID == "" {
		// Some backends may apply deletion synchronously.
		return nil
	}
	return d.waitTaskDone(ctx, taskID)
}

func (d *GuangYaPan) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}

	fileID := strings.TrimSpace(srcObj.GetID())
	if fileID == "" {
		return errors.New("file id is empty")
	}
	parentID := dstDir.GetID()

	var out deleteResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/move_file", map[string]any{
		"fileIds":  []string{fileID},
		"parentId": parentID,
	}, &out); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(out.Msg), "success") {
		return fmt.Errorf("move failed: %s", strings.TrimSpace(out.Msg))
	}
	taskID := strings.TrimSpace(out.Data.TaskID)
	if taskID == "" {
		return nil
	}
	return d.waitTaskDone(ctx, taskID)
}

func (d *GuangYaPan) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}

	fileID := strings.TrimSpace(srcObj.GetID())
	if fileID == "" {
		return errors.New("file id is empty")
	}
	parentID := dstDir.GetID()

	var out deleteResp
	if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/copy_file", map[string]any{
		"fileIds":  []string{fileID},
		"parentId": parentID,
	}, &out); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(out.Msg), "success") {
		return fmt.Errorf("copy failed: %s", strings.TrimSpace(out.Msg))
	}
	taskID := strings.TrimSpace(out.Data.TaskID)
	if taskID == "" {
		return nil
	}
	return d.waitTaskDone(ctx, taskID)
}

func (d *GuangYaPan) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	if err := d.ensureAccessToken(ctx); err != nil {
		return err
	}
	if file == nil {
		return errors.New("file is nil")
	}
	if file.GetSize() < 0 {
		return errors.New("invalid file size")
	}
	name := strings.TrimSpace(file.GetName())
	if name == "" {
		return errors.New("file name is empty")
	}

	parentID := dstDir.GetID()

	token, code, err := d.getUploadToken(ctx, parentID, name, file.GetSize())
	if err != nil {
		return err
	}
	taskID := strings.TrimSpace(token.TaskID)
	if code == 156 {
		if taskID == "" {
			return errors.New("instant upload returns empty task id")
		}
		return d.waitUploadTaskInfo(ctx, taskID)
	}

	if token.ObjectPath == "" || token.BucketName == "" || token.EndPoint == "" || token.AccessKeyID == "" || token.SecretAccessKey == "" {
		return errors.New("upload token is incomplete")
	}

	ossEndpoint := normalizeOSSEndpoint(token.EndPoint, token.BucketName)
	client, err := oss.New(ossEndpoint, token.AccessKeyID, token.SecretAccessKey, oss.SecurityToken(token.SessionToken))
	if err != nil {
		return fmt.Errorf("create oss client failed: %w", err)
	}
	bucket, err := client.Bucket(token.BucketName)
	if err != nil {
		return fmt.Errorf("create oss bucket failed: %w", err)
	}

	if file.GetSize() == 0 {
		if err := bucket.PutObject(token.ObjectPath, strings.NewReader("")); err != nil {
			return err
		}
	} else {
		if err := d.multipartUploadToOSS(ctx, bucket, token.ObjectPath, file, up); err != nil {
			return err
		}
	}

	if taskID == "" {
		return nil
	}
	return d.waitUploadTaskInfo(ctx, taskID)
}

func (d *GuangYaPan) getRootFolderID(ctx context.Context) (string, error) {
	if d.rootFolderResolved {
		return d.resolvedRootFolderID, nil
	}
	if err := d.ensureAccessToken(ctx); err != nil {
		return "", err
	}
	if err := d.prepareRootFolder(ctx); err != nil {
		return "", err
	}
	return d.resolvedRootFolderID, nil
}

func (d *GuangYaPan) prepareRootFolder(ctx context.Context) error {
	rootID, err := d.resolveConfiguredRootFolderID(ctx)
	if err != nil {
		return err
	}
	d.resolvedRootFolderID = rootID
	d.rootFolderResolved = true
	return nil
}

func (d *GuangYaPan) resolveConfiguredRootFolderID(ctx context.Context) (string, error) {
	root := strings.TrimSpace(d.RootPath)
	if root == "" {
		return "", nil
	}
	return d.resolveFolderPath(ctx, root)
}

func (d *GuangYaPan) resolveFolderPath(ctx context.Context, rootPath string) (string, error) {
	cleanPath := strings.Trim(strings.ReplaceAll(strings.TrimSpace(rootPath), "\\", "/"), "/")
	if cleanPath == "" {
		return "", nil
	}

	parentID := ""
	for _, name := range strings.Split(cleanPath, "/") {
		if name == "" {
			continue
		}
		childID, err := d.findChildFolderID(ctx, parentID, name)
		if err != nil {
			return "", err
		}
		parentID = childID
	}
	return parentID, nil
}

func (d *GuangYaPan) findChildFolderID(ctx context.Context, parentID, name string) (string, error) {
	pageSize := d.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	seen := 0
	for page := 0; ; page++ {
		var resp listResp
		body := map[string]any{
			"parentId":  parentID,
			"page":      page,
			"pageSize":  pageSize,
			"orderBy":   d.OrderBy,
			"sortType":  d.SortType,
			"fileTypes": []int{},
		}
		if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/get_file_list", body, &resp); err != nil {
			return "", err
		}
		for _, item := range resp.Data.List {
			seen++
			if item.ResType == 2 && item.FileName == name {
				return item.FileID, nil
			}
		}
		if len(resp.Data.List) < pageSize {
			break
		}
		if resp.Data.Total > 0 && seen >= resp.Data.Total {
			break
		}
	}

	if parentID == "" {
		return "", fmt.Errorf("resolve root folder path failed: folder %q not found under /", name)
	}
	return "", fmt.Errorf("resolve root folder path failed: folder %q not found under parent %s", name, parentID)
}

func (d *GuangYaPan) ensureAccessToken(ctx context.Context) error {
	if strings.TrimSpace(d.AccessToken) != "" {
		return nil
	}
	if strings.TrimSpace(d.RefreshToken) == "" {
		if d.canSMSLogin() {
			return d.loginBySMSCode(ctx)
		}
		if d.PhoneNumber != "" {
			return errors.New("not logged in yet: please fill verify_code and save storage to finish SMS login")
		}
		return errors.New("access token is empty")
	}
	return d.refreshToken(ctx)
}

func (d *GuangYaPan) validateToken(ctx context.Context) error {
	var me userMeResp
	resp, err := d.accountClient.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+d.AccessToken).
		SetResult(&me).
		Get("/v1/user/me")
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("validate token failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	if strings.TrimSpace(me.Sub) == "" {
		return errors.New("validate token failed: empty user sub")
	}
	return nil
}

func (d *GuangYaPan) refreshToken(ctx context.Context) error {
	if strings.TrimSpace(d.RefreshToken) == "" {
		return errors.New("refresh_token is empty")
	}

	var out tokenResp
	resp, err := d.accountClient.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"client_id":     d.ClientID,
			"grant_type":    "refresh_token",
			"refresh_token": d.RefreshToken,
		}).
		SetResult(&out).
		Post("/v1/auth/token")
	if err != nil {
		return err
	}
	if resp.IsError() || out.Error != "" || strings.TrimSpace(out.AccessToken) == "" {
		errMsg := strings.TrimSpace(out.ErrorDesc)
		if errMsg == "" {
			errMsg = strings.TrimSpace(out.Error)
		}
		if errMsg == "" {
			errMsg = strings.TrimSpace(resp.String())
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("status=%d", resp.StatusCode())
		}
		return fmt.Errorf("refresh token failed: %s", errMsg)
	}

	d.AccessToken = strings.TrimSpace(out.AccessToken)
	if strings.TrimSpace(out.RefreshToken) != "" {
		d.RefreshToken = strings.TrimSpace(out.RefreshToken)
	}
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *GuangYaPan) canSMSLogin() bool {
	return d.PhoneNumber != "" && d.VerifyCode != ""
}

func (d *GuangYaPan) loginBySMSCode(ctx context.Context) error {
	verificationID := strings.TrimSpace(d.VerificationID)
	if verificationID == "" {
		var err error
		verificationID, err = d.requestVerificationID(ctx)
		if err != nil {
			return err
		}
	}

	var step2 verifyResp
	resp, err := d.accountClient.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"verification_id":   verificationID,
			"verification_code": d.VerifyCode,
			"client_id":         d.ClientID,
		}).
		SetResult(&step2).
		Post("/v1/auth/verification/verify")
	if err != nil {
		return err
	}
	if resp.IsError() || step2.Error != "" || strings.TrimSpace(step2.VerificationToken) == "" {
		return fmt.Errorf("verify code failed: %s", d.accountErr(step2.ErrorDesc, step2.Error, resp))
	}

	var out tokenResp
	resp, err = d.accountClient.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"verification_code":  d.VerifyCode,
			"verification_token": step2.VerificationToken,
			"username":           normalizePhoneE164(d.PhoneNumber),
			"client_id":          d.ClientID,
		}).
		SetResult(&out).
		Post("/v1/auth/signin")
	if err != nil {
		return err
	}
	if resp.IsError() || out.Error != "" || strings.TrimSpace(out.AccessToken) == "" {
		return fmt.Errorf("signin failed: %s", d.accountErr(out.ErrorDesc, out.Error, resp))
	}

	d.AccessToken = strings.TrimSpace(out.AccessToken)
	d.RefreshToken = strings.TrimSpace(out.RefreshToken)
	d.VerificationID = ""
	// One-time SMS code should not be reused after successful login.
	d.VerifyCode = ""
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *GuangYaPan) prepareSMSCode(ctx context.Context) error {
	// Explicit send action should always refresh verification_id.
	d.VerificationID = ""
	if err := d.ensureCaptchaToken(ctx, false); err != nil {
		return err
	}
	verificationID, err := d.requestVerificationID(ctx)
	if err != nil {
		return err
	}
	d.VerificationID = verificationID
	d.SendCode = false
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *GuangYaPan) requestVerificationID(ctx context.Context) (string, error) {
	if d.CaptchaToken != "" {
		d.accountClient.SetHeader("X-Captcha-Token", d.CaptchaToken)
	}

	var step1 verificationResp
	resp, err := d.accountClient.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"phone_number": normalizePhoneE164(d.PhoneNumber),
			"target":       "ANY",
			"client_id":    d.ClientID,
		}).
		SetResult(&step1).
		Post("/v1/auth/verification")
	if err != nil {
		return "", err
	}
	if resp.IsError() || step1.Error != "" || strings.TrimSpace(step1.VerificationID) == "" {
		// If captcha token is expired/invalid, refresh it once and retry.
		if strings.Contains(step1.Error, "captcha_invalid") || strings.Contains(step1.ErrorDesc, "captcha_token expired") {
			if err := d.ensureCaptchaToken(ctx, true); err == nil {
				return d.requestVerificationID(ctx)
			}
		}
		return "", fmt.Errorf("request verification failed: %s", d.accountErr(step1.ErrorDesc, step1.Error, resp))
	}
	return strings.TrimSpace(step1.VerificationID), nil
}

func (d *GuangYaPan) ensureCaptchaToken(ctx context.Context, force bool) error {
	if !force && d.CaptchaToken != "" {
		d.accountClient.SetHeader("X-Captcha-Token", d.CaptchaToken)
		return nil
	}

	var out captchaInitResp
	resp, err := d.accountClient.R().
		SetContext(ctx).
		SetBody(map[string]any{
			"client_id": d.ClientID,
			"action":    "POST:/v1/auth/verification",
			"device_id": d.DeviceID,
			"meta": map[string]any{
				"username":           normalizePhoneE164(d.PhoneNumber),
				"phone_number":       normalizePhoneE164(d.PhoneNumber),
				"VERIFICATION_PHONE": normalizePhoneE164(d.PhoneNumber),
			},
		}).
		SetResult(&out).
		Post("/v1/shield/captcha/init")
	if err != nil {
		return err
	}
	if resp.IsError() || out.Error != "" || strings.TrimSpace(out.CaptchaToken) == "" {
		return fmt.Errorf("init captcha token failed: %s", d.accountErr(out.ErrorDesc, out.Error, resp))
	}
	d.CaptchaToken = strings.TrimSpace(out.CaptchaToken)
	d.accountClient.SetHeader("X-Captcha-Token", d.CaptchaToken)
	op.MustSaveDriverStorage(d)
	return nil
}

func normalizeCaptchaUsername(phone string) string {
	p := strings.TrimSpace(phone)
	p = strings.ReplaceAll(p, " ", "")
	p = strings.TrimPrefix(p, "+")
	// Keep only digits.
	b := make([]rune, 0, len(p))
	for _, ch := range p {
		if ch >= '0' && ch <= '9' {
			b = append(b, ch)
		}
	}
	digits := string(b)
	// Mainland number normalization: +86xxxxxxxxxxx -> xxxxxxxxxxx
	if strings.HasPrefix(digits, "86") && len(digits) > 11 {
		digits = digits[2:]
	}
	return digits
}

func normalizePhoneE164(phone string) string {
	p := strings.TrimSpace(phone)
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, " ", "")
	if strings.HasPrefix(p, "+") {
		// Format as "+86 1xxxxxxxxxx" to match browser payload expectations.
		if strings.HasPrefix(p, "+86") && len(p) > 3 {
			rest := strings.TrimPrefix(p, "+86")
			return "+86 " + rest
		}
		return p
	}
	// If raw mainland number is provided, normalize with +86 prefix.
	digits := normalizeCaptchaUsername(p)
	if len(digits) == 11 {
		return "+86 " + digits
	}
	return p
}

func (d *GuangYaPan) setTempStatus(status string) {
	// initStorage sets status to WORK after Init returns, so we update it shortly after.
	time.AfterFunc(200*time.Millisecond, func() {
		d.GetStorage().SetStatus(status)
		op.MustSaveDriverStorage(d)
	})
}

func (d *GuangYaPan) accountErr(desc, short string, resp *resty.Response) string {
	msg := strings.TrimSpace(desc)
	if msg == "" {
		msg = strings.TrimSpace(short)
	}
	if msg == "" && resp != nil {
		msg = strings.TrimSpace(resp.String())
	}
	if msg == "" && resp != nil {
		msg = fmt.Sprintf("status=%d", resp.StatusCode())
	}
	if msg == "" {
		msg = "unknown error"
	}
	return msg
}

func (d *GuangYaPan) postAPI(ctx context.Context, path string, body any, out any) error {
	if strings.TrimSpace(d.AccessToken) == "" {
		return errors.New("access token is empty")
	}
	resp, err := d.apiClient.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+d.AccessToken).
		SetBody(body).
		SetResult(out).
		Post(path)
	if err != nil {
		return err
	}
	if resp.StatusCode() == 401 || resp.StatusCode() == 403 {
		if strings.TrimSpace(d.RefreshToken) == "" {
			return fmt.Errorf("request failed: status=%d body=%s", resp.StatusCode(), resp.String())
		}
		if err := d.refreshToken(ctx); err != nil {
			return err
		}
		resp, err = d.apiClient.R().
			SetContext(ctx).
			SetHeader("Authorization", "Bearer "+d.AccessToken).
			SetBody(body).
			SetResult(out).
			Post(path)
		if err != nil {
			return err
		}
	}
	if resp.IsError() {
		return fmt.Errorf("request failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	return nil
}

func (d *GuangYaPan) waitTaskDone(ctx context.Context, taskID string) error {
	const (
		maxTry   = 30
		interval = 300 * time.Millisecond
	)
	for i := 0; i < maxTry; i++ {
		var out taskStatusResp
		if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/get_task_status", map[string]any{
			"taskId": taskID,
		}, &out); err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(out.Msg), "success") {
			return fmt.Errorf("get task status failed: %s", strings.TrimSpace(out.Msg))
		}
		switch out.Data.Status {
		case 2:
			return nil
		case -1, 3:
			return fmt.Errorf("task %s failed with status=%d", taskID, out.Data.Status)
		}
		if i == maxTry-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("task %s timeout", taskID)
}

func (d *GuangYaPan) getUploadToken(ctx context.Context, parentID, name string, size int64) (*uploadTokenData, int, error) {
	var out uploadTokenResp
	err := d.postAPI(ctx, "/nd.bizuserres.s/v1/get_res_center_token", map[string]any{
		"capacity": 2,
		"name":     name,
		"parentId": parentID,
		"res": map[string]any{
			"fileSize": size,
		},
	}, &out)
	if err != nil {
		return nil, 0, err
	}
	msg := strings.TrimSpace(out.Msg)
	if msg != "" && !strings.EqualFold(msg, "success") {
		return nil, out.Code, fmt.Errorf("get upload token failed: %s", msg)
	}
	if out.Data.TaskID == "" {
		return nil, out.Code, errors.New("get upload token failed: empty task id")
	}
	if out.Data.AccessKeyID == "" {
		out.Data.AccessKeyID = out.Data.Creds.AccessKeyID
	}
	if out.Data.SecretAccessKey == "" {
		out.Data.SecretAccessKey = out.Data.Creds.SecretAccessKey
	}
	if out.Data.SessionToken == "" {
		out.Data.SessionToken = out.Data.Creds.SessionToken
	}
	if strings.TrimSpace(out.Data.EndPoint) == "" {
		out.Data.EndPoint = strings.TrimSpace(out.Data.FullEndPoint)
	}
	if strings.TrimSpace(out.Data.EndPoint) != "" && !strings.HasPrefix(out.Data.EndPoint, "http://") && !strings.HasPrefix(out.Data.EndPoint, "https://") {
		if strings.TrimSpace(out.Data.FullEndPoint) != "" {
			out.Data.EndPoint = strings.TrimSpace(out.Data.FullEndPoint)
		} else if strings.TrimSpace(out.Data.BucketName) != "" {
			host := strings.TrimSpace(out.Data.EndPoint)
			prefix := strings.TrimSpace(out.Data.BucketName) + "."
			if strings.HasPrefix(host, prefix) {
				out.Data.EndPoint = "https://" + host
			} else {
				out.Data.EndPoint = "https://" + strings.TrimSpace(out.Data.BucketName) + "." + host
			}
		} else {
			out.Data.EndPoint = "https://" + strings.TrimSpace(out.Data.EndPoint)
		}
	}
	return &out.Data, out.Code, nil
}

func (d *GuangYaPan) waitUploadTaskInfo(ctx context.Context, taskID string) error {
	const (
		maxTry   = 300
		interval = 1 * time.Second
	)
	for i := 0; i < maxTry; i++ {
		var out taskInfoResp
		if err := d.postAPI(ctx, "/nd.bizuserres.s/v1/file/get_info_by_task_id", map[string]any{
			"taskId": taskID,
		}, &out); err != nil {
			return err
		}
		if out.Data.FileID != "" {
			return nil
		}
		switch out.Code {
		case 145, 146, 147, 155, 163, 0:
			// uploading/verifying/processing
		default:
			if strings.TrimSpace(out.Msg) != "" {
				return fmt.Errorf("upload task failed: code=%d msg=%s", out.Code, strings.TrimSpace(out.Msg))
			}
		}
		if i == maxTry-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("upload task %s timeout", taskID)
}

func (d *GuangYaPan) multipartUploadToOSS(ctx context.Context, bucket *oss.Bucket, objectPath string, file model.FileStreamer, up driver.UpdateProgress) error {
	partSize := calcUploadPartSize(file.GetSize())
	imur, err := bucket.InitiateMultipartUpload(objectPath, oss.Sequential())
	if err != nil {
		return err
	}

	total := file.GetSize()
	partCount := int((total + partSize - 1) / partSize)
	parts := make([]oss.UploadPart, 0, partCount)
	var uploaded int64
	partNumber := 1

	for uploaded < total {
		if err := ctx.Err(); err != nil {
			return err
		}
		curPartSize := partSize
		left := total - uploaded
		if left < curPartSize {
			curPartSize = left
		}

		reader := io.LimitReader(file, curPartSize)
		part, err := bucket.UploadPart(imur, driver.NewLimitedUploadStream(ctx, reader), curPartSize, partNumber)
		if err != nil {
			return err
		}
		parts = append(parts, part)
		uploaded += curPartSize
		partNumber++
		if total > 0 {
			up(100 * float64(uploaded) / float64(total))
		}
	}

	_, err = bucket.CompleteMultipartUpload(imur, parts)
	return err
}

func calcUploadPartSize(size int64) int64 {
	const (
		mb = int64(1024 * 1024)
		gb = int64(1024 * 1024 * 1024)
	)
	switch {
	case size <= 100*mb:
		return 1 * mb
	case size <= 16*gb:
		return 2 * mb
	case size <= 160*gb:
		return 4 * mb
	default:
		return 8 * mb
	}
}

func normalizeOSSEndpoint(endpoint, bucket string) string {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		return ep
	}
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		ep = "https://" + ep
	}
	u, err := url.Parse(ep)
	if err != nil || u.Host == "" {
		return ep
	}
	host := u.Host
	prefix := strings.TrimSpace(bucket)
	if prefix != "" && strings.HasPrefix(host, prefix+".") {
		host = strings.TrimPrefix(host, prefix+".")
	}
	u.Host = host
	return u.String()
}

func normalizeDeviceID(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "-", "")
	if len(v) != 32 {
		return ""
	}
	for _, ch := range v {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ""
		}
	}
	return v
}

func randomDeviceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}

var _ driver.Driver = (*GuangYaPan)(nil)
var _ driver.GetRooter = (*GuangYaPan)(nil)