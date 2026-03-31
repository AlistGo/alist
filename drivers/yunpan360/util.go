package yunpan360

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

const (
	baseURL          = "https://www.yunpan.com"
	indexPath        = "/file/index"
	listPath         = "/file/list"
	downloadPath     = "/file/download"
	openAPIProdURL   = "https://openapi.eyun.360.cn/intf.php"
	openAPITestURL   = "https://qaopen.eyun.360.cn/intf.php"
	openAPIHGTestURL = "https://hg-openapi.eyun.360.cn/intf.php"
)

func (d *Yunpan360) listPage(ctx context.Context, dirPath string, page, pageSize int) (ListResp, error) {
	if d.authMode() == authTypeAPIKey {
		return d.listOpenPage(ctx, dirPath, page, pageSize)
	}
	return d.listCookiePage(ctx, dirPath, page, pageSize)
}

func (d *Yunpan360) listCookiePage(ctx context.Context, dirPath string, page, pageSize int) (*CookieListResp, error) {
	var resp CookieListResp
	err := d.cookieRequestForm(ctx, listPath, map[string]string{
		"path":      requestPath(dirPath),
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
		"order":     requestOrder(d.OrderDirection),
		"field":     "file_name",
		"focus_nid": "0",
	}, &resp)
	if err != nil {
		return nil, err
	}
	d.cacheCookieDownloadSession(resp.GetOwnerQID(), resp.Token)
	return &resp, nil
}

func (d *Yunpan360) cookieRequestForm(ctx context.Context, apiPath string, form map[string]string, out interface{}) error {
	req := base.RestyClient.R().
		SetContext(ctx).
		SetHeaders(map[string]string{
			"Accept":           "text/javascript, text/html, application/xml, text/xml, */*",
			"Content-Type":     "application/x-www-form-urlencoded",
			"Cookie":           d.Cookie,
			"Origin":           baseURL,
			"Referer":          baseURL + "/file/index",
			"X-Requested-With": "XMLHttpRequest",
		}).
		SetFormData(form)

	res, err := req.Execute(http.MethodPost, baseURL+apiPath)
	if err != nil {
		return err
	}

	var baseResp BaseResp
	if err := utils.Json.Unmarshal(res.Body(), &baseResp); err != nil {
		return err
	}
	if baseResp.Errno != 0 {
		if baseResp.Errmsg == "" {
			return fmt.Errorf("yunpan request failed: errno=%d", baseResp.Errno)
		}
		return errors.New(baseResp.Errmsg)
	}
	if out == nil {
		return nil
	}
	return utils.Json.Unmarshal(res.Body(), out)
}

func requestPath(dirPath string) string {
	path := normalizeRemotePath(dirPath)
	if path == "" {
		return "/"
	}
	return path
}

func requestOrder(order string) string {
	if strings.EqualFold(order, "desc") {
		return "desc"
	}
	return "asc"
}

func (d *Yunpan360) cookiePage(ctx context.Context, pagePath string) ([]byte, error) {
	req := base.RestyClient.R().
		SetContext(ctx).
		SetHeaders(map[string]string{
			"Accept":  "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Cookie":  d.Cookie,
			"Referer": baseURL + indexPath,
		})
	res, err := req.Get(baseURL + pagePath)
	if err != nil {
		return nil, err
	}
	return res.Body(), nil
}

func openAPIURL(env string) string {
	switch env {
	case "test":
		return openAPITestURL
	case "hgtest":
		return openAPIHGTestURL
	default:
		return openAPIProdURL
	}
}

func openClientSecretForEnv(env string) string {
	if env == "test" {
		return openClientSecretQA
	}
	return openClientSecret
}

func phpQueryEscape(raw string) string {
	escaped := url.QueryEscape(raw)
	return strings.ReplaceAll(escaped, "~", "%7E")
}

func openSign(accessToken, qid, method string, extra map[string]string) string {
	params := map[string]string{
		"access_token": accessToken,
		"method":       method,
		"qid":          qid,
	}
	for key, value := range extra {
		if value != "" {
			params[key] = value
		}
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sortStrings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+phpQueryEscape(params[key]))
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + openSignSecret))
	return hex.EncodeToString(sum[:])
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func (d *Yunpan360) getOpenAuth(ctx context.Context) (*OpenAuthInfo, error) {
	d.authMu.Lock()
	defer d.authMu.Unlock()

	if d.cachedOpenAuth != nil && time.Now().Before(d.openAuthExpire) {
		auth := *d.cachedOpenAuth
		return &auth, nil
	}

	reqURL := openAPIURL(d.EcsEnv)
	req := base.RestyClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("api_key", d.APIKey).
		SetQueryParams(map[string]string{
			"method":        "Oauth.getAccessTokenByApiKey",
			"client_id":     openClientID,
			"client_secret": openClientSecretForEnv(d.EcsEnv),
			"grant_type":    "authorization_code",
			"sub_channel":   d.SubChannel,
			"api_key":       d.APIKey,
		})

	res, err := req.Get(reqURL)
	if err != nil {
		return nil, err
	}

	var resp OpenAuthResp
	if err := utils.Json.Unmarshal(res.Body(), &resp); err != nil {
		return nil, err
	}
	if resp.Errno != 0 {
		if resp.Errmsg == "" {
			return nil, fmt.Errorf("yunpan auth failed: errno=%d", resp.Errno)
		}
		return nil, errors.New(resp.Errmsg)
	}

	auth := &OpenAuthInfo{
		AccessToken: resp.Data.AccessToken,
		Qid:         resp.Data.Qid,
		Token:       resp.Data.Token,
		SubChannel:  d.SubChannel,
	}
	d.cachedOpenAuth = auth
	d.openAuthExpire = time.Now().Add(50 * time.Minute)

	copied := *auth
	return &copied, nil
}

func (d *Yunpan360) openBaseParams(auth *OpenAuthInfo, method string, signParams map[string]string, withSign bool) map[string]string {
	params := map[string]string{
		"method":       method,
		"access_token": auth.AccessToken,
		"qid":          auth.Qid,
		"sub_channel":  auth.SubChannel,
	}
	if withSign {
		params["sign"] = openSign(auth.AccessToken, auth.Qid, method, signParams)
	} else {
		params["sign"] = ""
	}
	return params
}

func (d *Yunpan360) openGET(ctx context.Context, method string, signParams map[string]string, query map[string]string, out interface{}, withSign bool) error {
	auth, err := d.getOpenAuth(ctx)
	if err != nil {
		return err
	}
	params := d.openBaseParams(auth, method, signParams, withSign)
	for key, value := range query {
		params[key] = value
	}

	req := base.RestyClient.R().
		SetContext(ctx).
		SetHeader("Access-Token", auth.AccessToken).
		SetQueryParams(params)
	res, err := req.Get(openAPIURL(d.EcsEnv))
	if err != nil {
		return err
	}
	return decodeBaseResp(res.Body(), out)
}

func (d *Yunpan360) openPOST(ctx context.Context, method string, signParams map[string]string, query, body map[string]string, out interface{}, withSign bool) error {
	auth, err := d.getOpenAuth(ctx)
	if err != nil {
		return err
	}
	queryParams := map[string]string{}
	for key, value := range query {
		queryParams[key] = value
	}
	bodyParams := map[string]string{}
	for key, value := range body {
		bodyParams[key] = value
	}

	baseParams := d.openBaseParams(auth, method, signParams, withSign)
	if len(queryParams) == 0 {
		bodyParams = mergeStringMaps(baseParams, bodyParams)
	} else {
		queryParams = mergeStringMaps(baseParams, queryParams)
	}

	req := base.RestyClient.R().
		SetContext(ctx).
		SetHeader("Access-Token", auth.AccessToken).
		SetHeader("Content-Type", "application/x-www-form-urlencoded")
	if len(queryParams) > 0 {
		req.SetQueryParams(queryParams)
	}
	if len(bodyParams) > 0 {
		req.SetFormData(bodyParams)
	}
	res, err := req.Post(openAPIURL(d.EcsEnv))
	if err != nil {
		return err
	}
	return decodeBaseResp(res.Body(), out)
}

func decodeBaseResp(body []byte, out interface{}) error {
	var baseResp BaseResp
	if err := utils.Json.Unmarshal(body, &baseResp); err != nil {
		return err
	}
	if baseResp.Errno != 0 {
		if baseResp.Errmsg == "" {
			return fmt.Errorf("yunpan request failed: errno=%d", baseResp.Errno)
		}
		return errors.New(baseResp.Errmsg)
	}
	if out == nil {
		return nil
	}
	return utils.Json.Unmarshal(body, out)
}

func mergeStringMaps(baseMap, extra map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range baseMap {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

func (d *Yunpan360) cookieDownloadURL(ctx context.Context, file model.Obj) (*CookieDownloadResp, error) {
	resp, err := d.cookieDownloadURLOnce(ctx, file, false)
	if err == nil {
		return resp, nil
	}

	d.invalidateCookieDownloadSession()
	return d.cookieDownloadURLOnce(ctx, file, true)
}

func (d *Yunpan360) cookieDownloadURLOnce(ctx context.Context, file model.Obj, refresh bool) (*CookieDownloadResp, error) {
	nid := strings.TrimSpace(file.GetID())
	if nid == "" {
		return nil, errors.New("missing file id")
	}

	fname := normalizeRemotePath(file.GetPath())
	if fname == "" {
		return nil, errors.New("missing file path")
	}

	ownerQID, token, err := d.resolveCookieDownloadParams(ctx, file, refresh)
	if err != nil {
		return nil, err
	}

	var resp CookieDownloadResp
	err = d.cookieRequestForm(ctx, downloadPath, map[string]string{
		"nid":       nid,
		"fname":     fname,
		"owner_qid": ownerQID,
		"token":     token,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) cookieRename(ctx context.Context, srcObj model.Obj, newName string) error {
	path := normalizeRemotePath(srcObj.GetPath())
	if path == "" {
		return errors.New("missing object path")
	}
	nid := strings.TrimSpace(srcObj.GetID())
	if nid == "" {
		return errors.New("missing object id")
	}

	ownerQID, err := d.resolveCookieOwnerQID(ctx, srcObj, false)
	if err != nil {
		return err
	}

	return d.cookieRequestForm(ctx, "/file/rename", map[string]string{
		"path":      path,
		"nid":       nid,
		"newpath":   strings.TrimSuffix(strings.TrimSpace(newName), "/"),
		"owner_qid": ownerQID,
	}, nil)
}

func (d *Yunpan360) resolveCookieDownloadParams(ctx context.Context, file model.Obj, refresh bool) (string, string, error) {
	ownerQID := sanitizeOwnerQID(d.OwnerQID)
	token := strings.TrimSpace(d.DownloadToken)

	if obj, ok := file.(*YunpanObject); ok {
		ownerQID = firstNonEmpty(sanitizeOwnerQID(obj.OwnerQID), ownerQID)
		token = firstNonEmpty(strings.TrimSpace(obj.DownloadToken), token)
	}

	ownerQID = firstNonEmpty(ownerQID,
		sanitizeOwnerQID(getCookieValue(d.Cookie, "owner_qid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "ownerQid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "qid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "QID")),
	)
	token = firstNonEmpty(token,
		getCookieValue(d.Cookie, "download_token"),
		getCookieValue(d.Cookie, "token"),
		getCookieValue(d.Cookie, "Token"),
	)

	if ownerQID == "" && token != "" {
		ownerQID = ownerQIDFromToken(token)
	}
	if ownerQID != "" && token != "" {
		return ownerQID, token, nil
	}

	if !refresh {
		if cached := d.getCachedCookieDownloadSession(); cached != nil {
			ownerQID = firstNonEmpty(ownerQID, cached.OwnerQID)
			token = firstNonEmpty(token, cached.Token)
		}
		if ownerQID == "" && token != "" {
			ownerQID = ownerQIDFromToken(token)
		}
		if ownerQID != "" && token != "" {
			return ownerQID, token, nil
		}
	}

	resp, err := d.listCookiePage(ctx, d.RootFolderPath, 0, 1)
	if err == nil && resp != nil {
		ownerQID = firstNonEmpty(ownerQID, resp.GetOwnerQID())
		token = firstNonEmpty(token, strings.TrimSpace(resp.Token))
	}
	if ownerQID == "" && token != "" {
		ownerQID = ownerQIDFromToken(token)
	}
	if ownerQID != "" && token != "" {
		d.cacheCookieDownloadSession(ownerQID, token)
		return ownerQID, token, nil
	}

	pageSession, err := d.getCookieDownloadSessionFromPage(ctx)
	if err == nil && pageSession != nil {
		ownerQID = firstNonEmpty(ownerQID, pageSession.OwnerQID)
		token = firstNonEmpty(token, pageSession.Token)
	}
	if ownerQID == "" && token != "" {
		ownerQID = ownerQIDFromToken(token)
	}
	if ownerQID == "" || token == "" {
		return "", "", errors.New("missing owner_qid or download_token for cookie mode")
	}

	d.cacheCookieDownloadSession(ownerQID, token)
	return ownerQID, token, nil
}

func (d *Yunpan360) resolveCookieOwnerQID(ctx context.Context, file model.Obj, refresh bool) (string, error) {
	ownerQID := sanitizeOwnerQID(d.OwnerQID)

	if obj, ok := file.(*YunpanObject); ok {
		ownerQID = firstNonEmpty(sanitizeOwnerQID(obj.OwnerQID), ownerQID)
	}

	ownerQID = firstNonEmpty(ownerQID,
		sanitizeOwnerQID(getCookieValue(d.Cookie, "owner_qid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "ownerQid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "qid")),
		sanitizeOwnerQID(getCookieValue(d.Cookie, "QID")),
	)
	if ownerQID != "" {
		return ownerQID, nil
	}

	if !refresh {
		if cached := d.getCachedCookieDownloadSession(); cached != nil {
			ownerQID = firstNonEmpty(ownerQID, cached.OwnerQID)
		}
		if ownerQID != "" {
			return ownerQID, nil
		}
	}

	resp, err := d.listCookiePage(ctx, d.RootFolderPath, 0, 1)
	if err == nil && resp != nil {
		ownerQID = firstNonEmpty(ownerQID, resp.GetOwnerQID())
	}
	if ownerQID != "" {
		return ownerQID, nil
	}

	pageSession, err := d.getCookieDownloadSessionFromPage(ctx)
	if err == nil && pageSession != nil {
		ownerQID = firstNonEmpty(ownerQID, pageSession.OwnerQID)
	}
	if ownerQID == "" {
		return "", errors.New("missing owner_qid for cookie mode")
	}
	return ownerQID, nil
}

func (d *Yunpan360) getCookieDownloadSessionFromPage(ctx context.Context) (*CookieDownloadSession, error) {
	body, err := d.cookiePage(ctx, indexPath)
	if err != nil {
		return nil, err
	}
	session := parseCookieDownloadSessionFromText(string(body))
	if session == nil {
		return nil, errors.New("failed to parse cookie download session from page")
	}
	d.cacheCookieSession(session)
	return session, nil
}

func (d *Yunpan360) getCachedCookieDownloadSession() *CookieDownloadSession {
	d.authMu.Lock()
	defer d.authMu.Unlock()

	if d.cachedCookieSession == nil || time.Now().After(d.cookieSessionExpire) {
		return nil
	}
	session := *d.cachedCookieSession
	return &session
}

func (d *Yunpan360) cacheCookieDownloadSession(ownerQID, token string) {
	d.cacheCookieSession(&CookieDownloadSession{
		OwnerQID: ownerQID,
		Token:    token,
	})
}

func (d *Yunpan360) cacheCookieSession(session *CookieDownloadSession) {
	if session == nil {
		return
	}

	cached := &CookieDownloadSession{
		OwnerQID: sanitizeOwnerQID(session.OwnerQID),
		Token:    strings.TrimSpace(session.Token),
	}
	if cached.OwnerQID == "" && cached.Token != "" {
		cached.OwnerQID = ownerQIDFromToken(cached.Token)
	}
	if cached.OwnerQID == "" || cached.Token == "" {
		return
	}

	d.authMu.Lock()
	defer d.authMu.Unlock()

	d.cachedCookieSession = cached
	d.cookieSessionExpire = time.Now().Add(10 * time.Minute)
}

func (d *Yunpan360) invalidateCookieDownloadSession() {
	d.authMu.Lock()
	defer d.authMu.Unlock()

	d.cachedCookieSession = nil
	d.cookieSessionExpire = time.Time{}
}

func getCookieValue(rawCookie, name string) string {
	for _, item := range strings.Split(rawCookie, ";") {
		part := strings.TrimSpace(item)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok || key != name {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"")
		unescaped, err := url.QueryUnescape(value)
		if err == nil {
			return strings.TrimSpace(unescaped)
		}
		return value
	}
	return ""
}

func ownerQIDFromToken(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 4 {
		return ""
	}
	qid := strings.TrimSpace(parts[3])
	for _, ch := range qid {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return qid
}

func sanitizeOwnerQID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return ""
	}
	return raw
}

func parseCookieDownloadSessionFromText(text string) *CookieDownloadSession {
	token := extractFirstMatch(text,
		`(?i)["']download_token["']\s*[:=]\s*["']([^"'<>]+)["']`,
		`(?i)["']token["']\s*[:=]\s*["']([^"'<>]+)["']`,
		`(?i)\btoken\s*[:=]\s*["']([^"'<>]+)["']`,
	)
	ownerQID := extractFirstMatch(text,
		`(?i)["']owner_qid["']\s*[:=]\s*["']?([0-9]+)["']?`,
		`(?i)["']ownerQid["']\s*[:=]\s*["']?([0-9]+)["']?`,
		`(?i)["']qid["']\s*[:=]\s*["']?([0-9]+)["']?`,
		`(?i)\bowner_qid\s*[:=]\s*["']?([0-9]+)["']?`,
		`(?i)\bqid\s*[:=]\s*["']?([0-9]+)["']?`,
	)
	if ownerQID == "" && token != "" {
		ownerQID = ownerQIDFromToken(token)
	}
	if ownerQID == "" || token == "" {
		return nil
	}
	return &CookieDownloadSession{
		OwnerQID: ownerQID,
		Token:    token,
	}
}

func extractFirstMatch(text string, patterns ...string) string {
	return extractFirstValidatedMatch(nil, text, patterns...)
}

func extractFirstValidatedMatch(validate func(string) bool, text string, patterns ...string) string {
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		for _, matches := range re.FindAllStringSubmatch(text, -1) {
			if len(matches) < 2 {
				continue
			}
			value := html.UnescapeString(strings.TrimSpace(matches[1]))
			value = strings.Trim(value, "\"'")
			if value == "" {
				continue
			}
			if validate == nil || validate(value) {
				return value
			}
		}
	}
	return ""
}

func (d *Yunpan360) listOpenPage(ctx context.Context, dirPath string, page, pageSize int) (*OpenListResp, error) {
	var resp OpenListResp
	path := ensureDirAPIPath(dirPath)
	params := map[string]string{
		"path":      path,
		"page":      strconv.Itoa(page),
		"page_size": strconv.Itoa(pageSize),
	}
	err := d.openGET(ctx, "File.getList", params, params, &resp, true)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openUserInfo(ctx context.Context) (*OpenUserInfoResp, error) {
	var resp OpenUserInfoResp
	err := d.openGET(ctx, "User.getUserDetail", nil, nil, &resp, false)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openDownloadURL(ctx context.Context, file model.Obj) (*OpenDownloadResp, error) {
	var resp OpenDownloadResp
	signParams := map[string]string{}
	body := map[string]string{}

	if file.GetPath() != "" {
		signParams["fpath"] = normalizeRemotePath(file.GetPath())
		body["fpath"] = signParams["fpath"]
	} else if file.GetID() != "" {
		signParams["nid"] = file.GetID()
		body["nid"] = file.GetID()
	} else {
		return nil, errors.New("missing file path and id")
	}

	err := d.openPOST(ctx, "MCP.getDownLoadUrl", signParams, nil, body, &resp, true)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) cookieMakeDir(ctx context.Context, fullPath string) (*CookieMkdirResp, error) {
	var resp CookieMkdirResp
	body := map[string]string{
		"path":      ensureDirAPIPath(fullPath),
		"owner_qid": "0",
	}
	err := d.cookieRequestForm(ctx, "/file/mkdir", body, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openMakeDir(ctx context.Context, fullPath string) (*OpenMkdirResp, error) {
	var resp OpenMkdirResp
	body := map[string]string{"fname": ensureDirAPIPath(fullPath)}
	err := d.openPOST(ctx, "File.mkdir", body, nil, body, &resp, true)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) openRename(ctx context.Context, srcName, newName string) error {
	signParams := map[string]string{
		"src_name": srcName,
		"new_name": newName,
	}
	return d.openPOST(ctx, "File.rename", signParams, nil, signParams, nil, true)
}

func (d *Yunpan360) cookieMove(ctx context.Context, srcPath, dstPath string) error {
	var resp CookieMoveResp
	body := map[string]string{
		"path[]":  srcPath,
		"newpath": ensureDirAPIPath(dstPath),
	}
	if err := d.cookieRequestForm(ctx, "/file/move", body, &resp); err != nil {
		return err
	}
	if !resp.Data.IsAsync {
		return nil
	}
	return d.waitCookieAsyncTask(ctx, resp.Data.TaskID)
}

func (d *Yunpan360) cookieRecycle(ctx context.Context, obj model.Obj) error {
	path := apiPathForObj(obj)
	if path == "" {
		return errors.New("missing object path")
	}
	ownerQID, err := d.resolveCookieOwnerQID(ctx, obj, false)
	if err != nil {
		return err
	}

	var resp CookieRecycleResp
	if err := d.cookieRequestForm(ctx, "/file/recycle", map[string]string{
		"path[]":    path,
		"owner_qid": ownerQID,
	}, &resp); err != nil {
		return err
	}
	if !resp.Data.IsAsync {
		return nil
	}
	return d.waitCookieAsyncTask(ctx, resp.Data.TaskID, 3008)
}

func (d *Yunpan360) cookieAsyncQuery(ctx context.Context, taskID string) (*CookieAsyncQueryResp, error) {
	var resp CookieAsyncQueryResp
	err := d.cookieRequestForm(ctx, "/async/query", map[string]string{
		"task_id": strings.TrimSpace(taskID),
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (d *Yunpan360) waitCookieAsyncTask(ctx context.Context, taskID string, toleratedErrnos ...int) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}

	tolerated := map[int]struct{}{}
	for _, errno := range toleratedErrnos {
		tolerated[errno] = struct{}{}
	}

	for attempt := 0; attempt < 15; attempt++ {
		resp, err := d.cookieAsyncQuery(ctx, taskID)
		if err == nil && resp != nil {
			if task, ok := resp.Data[taskID]; ok {
				done, taskErr := checkCookieAsyncTask(task, tolerated)
				if done {
					return taskErr
				}
			}
		}
		if attempt == 14 {
			break
		}
		if err := sleepWithContext(ctx, 300*time.Millisecond); err != nil {
			return err
		}
	}

	// Keep prior behavior when the async task is still pending after the probe window.
	return nil
}

func checkCookieAsyncTask(task CookieAsyncTask, toleratedErrnos map[int]struct{}) (bool, error) {
	if task.Status != 10 {
		return false, nil
	}
	if task.Errno == 0 {
		return true, nil
	}
	if _, ok := toleratedErrnos[task.Errno]; ok {
		return true, nil
	}
	if strings.TrimSpace(task.Errstr) != "" {
		return true, errors.New(task.Errstr)
	}
	if strings.TrimSpace(task.Action) != "" {
		return true, fmt.Errorf("yunpan async task %s failed: errno=%d", task.Action, task.Errno)
	}
	return true, fmt.Errorf("yunpan async task failed: errno=%d", task.Errno)
}

func (d *Yunpan360) openMove(ctx context.Context, srcName, dstPath string) error {
	signParams := map[string]string{
		"src_name": srcName,
		"new_name": dstPath,
	}
	return d.openPOST(ctx, "File.move", signParams, nil, signParams, nil, true)
}

func (d *Yunpan360) openDelete(ctx context.Context, targetPath string) error {
	return d.openPOST(ctx, "File.delete", nil, nil, map[string]string{
		"fname": targetPath,
	}, nil, true)
}

func apiPathForObj(obj model.Obj) string {
	if obj.IsDir() {
		return ensureDirAPIPath(obj.GetPath())
	}
	return normalizeRemotePath(obj.GetPath())
}

func ensureDirSuffix(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasSuffix(name, "/") {
		return name
	}
	return name + "/"
}

func ensureDirAPIPath(p string) string {
	p = normalizeRemotePath(p)
	if p == "" || p == "/" {
		return "/"
	}
	return p + "/"
}

func cloneObj(src model.Obj, newPath, newName string) model.Obj {
	obj := model.Object{
		ID:       src.GetID(),
		Path:     normalizeRemotePath(newPath),
		Name:     newName,
		Size:     src.GetSize(),
		Modified: src.ModTime(),
		Ctime:    src.CreateTime(),
		IsFolder: src.IsDir(),
		HashInfo: src.GetHash(),
	}
	if raw, ok := src.(*YunpanObject); ok {
		return &YunpanObject{
			Object:        obj,
			Thumbnail:     raw.Thumbnail,
			OwnerQID:      raw.OwnerQID,
			DownloadToken: raw.DownloadToken,
		}
	}
	return &obj
}

func absoluteURL(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if strings.HasPrefix(raw, "/") {
		return baseURL + raw
	}
	return baseURL + "/" + raw
}
