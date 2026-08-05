package _139

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/pkg/http_range"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	cookiepkg "github.com/alist-org/alist/v3/pkg/cookie"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
)

const (
	KEY_HEX_1 = "73634235495062495331515373756c734e7253306c673d3d"
	KEY_HEX_2 = "7150714477323633586746674c337538"
)

var mailLoginCookieOrder = []string{
	"behaviorid",
	"Os_SSo_Sid",
	"_139_index_isLoginType",
	"_139_login_version",
	"Login_UserNumber",
	"cookiepartid8011",
	"_139_login_agreement",
	"UserData",
	"rmUin8011",
	"cookiepartid",
	"UUIDToken",
	"SkinPath28011",
	"cbauto",
	"areaCode8011",
	"cookieLen",
	"DEVICE_INFO_DIGEST",
	"JSESSIONID",
	"loginProcessFlag",
	"provCode8011",
	"S_DEVICE_TOKEN",
	"taskIdCloud",
	"UserNowState",
	"UserNowState8011",
	"ut8011",
}

type credentialState int

const (
	credentialStateAuthorization credentialState = iota
	credentialStateFullLogin
	credentialStateCookiesOnly
)

// do others that not defined in Driver interface
func (d *Yun139) isFamily() bool {
	return d.Type == "family"
}

func encodeURIComponent(str string) string {
	r := url.QueryEscape(str)
	r = strings.Replace(r, "+", "%20", -1)
	r = strings.Replace(r, "%21", "!", -1)
	r = strings.Replace(r, "%27", "'", -1)
	r = strings.Replace(r, "%28", "(", -1)
	r = strings.Replace(r, "%29", ")", -1)
	r = strings.Replace(r, "%2A", "*", -1)
	return r
}

func calSign(body, ts, randStr string) string {
	body = encodeURIComponent(body)
	strs := strings.Split(body, "")
	sort.Strings(strs)
	body = strings.Join(strs, "")
	body = base64.StdEncoding.EncodeToString([]byte(body))
	res := utils.GetMD5EncodeStr(body) + utils.GetMD5EncodeStr(ts+":"+randStr)
	res = strings.ToUpper(utils.GetMD5EncodeStr(res))
	return res
}

func getTime(t string) time.Time {
	stamp, _ := time.ParseInLocation("20060102150405", t, utils.CNLoc)
	return stamp
}

func (d *Yun139) refreshToken() error {
	if d.ref != nil {
		return d.ref.refreshToken()
	}
	decode, err := base64.StdEncoding.DecodeString(d.Authorization)
	if err != nil {
		return fmt.Errorf("authorization decode failed: %s", err)
	}
	decodeStr := string(decode)
	splits := strings.Split(decodeStr, ":")
	if len(splits) < 3 {
		return fmt.Errorf("authorization is invalid, splits < 3")
	}
	d.Account = splits[1]
	strs := strings.Split(splits[2], "|")
	if len(strs) < 4 {
		return fmt.Errorf("authorization is invalid, strs < 4")
	}
	expiration, err := strconv.ParseInt(strs[3], 10, 64)
	if err != nil {
		return fmt.Errorf("authorization is invalid")
	}
	expiration -= time.Now().UnixMilli()
	if expiration > 1000*60*60*24*15 {
		// Authorization有效期大于15天无需刷新
		return nil
	}
	if expiration < 0 {
		return fmt.Errorf("authorization has expired")
	}

	url := "https://aas.caiyun.feixin.10086.cn:443/tellin/authTokenRefresh.do"
	var resp RefreshTokenResp
	reqBody := "<root><token>" + splits[2] + "</token><account>" + splits[1] + "</account><clienttype>656</clienttype></root>"
	_, err = base.RestyClient.R().
		ForceContentType("application/xml").
		SetBody(reqBody).
		SetResult(&resp).
		Post(url)
	if err != nil || resp.Return != "0" {
		state, stateErr := d.credentialState()
		if stateErr == nil && state == credentialStateFullLogin {
			log.Warnf("139yun: failed to refresh token with old token: %v, desc: %s. trying to login with password.", err, resp.Desc)
			_, loginErr := d.loginWithPassword()
			if loginErr != nil {
				return fmt.Errorf("failed to login with password after refresh failed: %w", loginErr)
			}
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("failed to refresh token: %s", resp.Desc)
	}
	d.Authorization = base64.StdEncoding.EncodeToString([]byte(splits[0] + ":" + splits[1] + ":" + resp.Token))
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *Yun139) request(pathname string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	url := "https://yun.139.com" + pathname
	req := base.RestyClient.R()
	randStr := random.String(16)
	ts := time.Now().Format("2006-01-02 15:04:05")
	if callback != nil {
		callback(req)
	}
	body, err := utils.Json.Marshal(req.Body)
	if err != nil {
		return nil, err
	}
	sign := calSign(string(body), ts, randStr)
	svcType := "1"
	if d.isFamily() {
		svcType = "2"
	}
	req.SetHeaders(map[string]string{
		"Accept":         "application/json, text/plain, */*",
		"CMS-DEVICE":     "default",
		"Authorization":  "Basic " + d.getAuthorization(),
		"mcloud-channel": "1000101",
		"mcloud-client":  "10701",
		//"mcloud-route": "001",
		"mcloud-sign": fmt.Sprintf("%s,%s,%s", ts, randStr, sign),
		//"mcloud-skey":"",
		"mcloud-version":         "7.14.0",
		"Origin":                 "https://yun.139.com",
		"Referer":                "https://yun.139.com/w/",
		"x-DeviceInfo":           "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||",
		"x-huawei-channelSrc":    "10000034",
		"x-inner-ntwk":           "2",
		"x-m4c-caller":           "PC",
		"x-m4c-src":              "10002",
		"x-SvcType":              svcType,
		"Inner-Hcy-Router-Https": "1",
	})

	var e BaseResp
	req.SetResult(&e)
	res, err := req.Execute(method, url)
	log.Debugln(res.String())
	if !e.Success {
		return nil, errors.New(e.Message)
	}
	if resp != nil {
		err = utils.Json.Unmarshal(res.Body(), resp)
		if err != nil {
			return nil, err
		}
	}
	return res.Body(), nil
}

func (d *Yun139) requestRoute(data interface{}, resp interface{}) ([]byte, error) {
	url := "https://user-njs.yun.139.com/user/route/qryRoutePolicy"
	req := base.RestyClient.R()
	randStr := random.String(16)
	ts := time.Now().Format("2006-01-02 15:04:05")
	callback := func(req *resty.Request) {
		req.SetBody(data)
	}
	if callback != nil {
		callback(req)
	}
	body, err := utils.Json.Marshal(req.Body)
	if err != nil {
		return nil, err
	}
	sign := calSign(string(body), ts, randStr)
	svcType := "1"
	if d.isFamily() {
		svcType = "2"
	}
	req.SetHeaders(map[string]string{
		"Accept":         "application/json, text/plain, */*",
		"CMS-DEVICE":     "default",
		"Authorization":  "Basic " + d.getAuthorization(),
		"mcloud-channel": "1000101",
		"mcloud-client":  "10701",
		//"mcloud-route": "001",
		"mcloud-sign": fmt.Sprintf("%s,%s,%s", ts, randStr, sign),
		//"mcloud-skey":"",
		"mcloud-version":         "7.14.0",
		"Origin":                 "https://yun.139.com",
		"Referer":                "https://yun.139.com/w/",
		"x-DeviceInfo":           "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||",
		"x-huawei-channelSrc":    "10000034",
		"x-inner-ntwk":           "2",
		"x-m4c-caller":           "PC",
		"x-m4c-src":              "10002",
		"x-SvcType":              svcType,
		"Inner-Hcy-Router-Https": "1",
	})

	var e BaseResp
	req.SetResult(&e)
	res, err := req.Execute(http.MethodPost, url)
	log.Debugln(res.String())
	if !e.Success {
		return nil, errors.New(e.Message)
	}
	if resp != nil {
		err = utils.Json.Unmarshal(res.Body(), resp)
		if err != nil {
			return nil, err
		}
	}
	return res.Body(), nil
}

func (d *Yun139) ensurePersonalCloudHost() error {
	if d.ref != nil {
		return d.ref.ensurePersonalCloudHost()
	}
	if d.PersonalCloudHost != "" {
		return nil
	}
	if len(d.Authorization) == 0 {
		return fmt.Errorf("authorization is empty")
	}
	if d.Account == "" {
		if err := d.refreshToken(); err != nil {
			return err
		}
	}

	var resp QueryRoutePolicyResp
	_, err := d.requestRoute(base.Json{
		"userInfo": base.Json{
			"userType":    1,
			"accountType": 1,
			"accountName": d.Account,
		},
		"modAddrType": 1,
	}, &resp)
	if err != nil {
		return err
	}
	for _, policyItem := range resp.Data.RoutePolicyList {
		if policyItem.ModName == "personal" && policyItem.HttpsUrl != "" {
			d.PersonalCloudHost = strings.TrimRight(policyItem.HttpsUrl, "/")
			break
		}
	}
	if d.PersonalCloudHost == "" {
		return fmt.Errorf("personal cloud host is empty")
	}
	return nil
}

func (d *Yun139) post(pathname string, data interface{}, resp interface{}) ([]byte, error) {
	return d.request(pathname, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, resp)
}

func (d *Yun139) getFiles(catalogID string) ([]model.Obj, error) {
	start := 0
	limit := 100
	files := make([]model.Obj, 0)
	for {
		data := base.Json{
			"catalogID":       catalogID,
			"sortDirection":   1,
			"startNumber":     start + 1,
			"endNumber":       start + limit,
			"filterType":      0,
			"catalogSortType": 0,
			"contentSortType": 0,
			"commonAccountInfo": base.Json{
				"account":     d.getAccount(),
				"accountType": 1,
			},
		}
		var resp GetDiskResp
		_, err := d.post("/orchestration/personalCloud/catalog/v1.0/getDisk", data, &resp)
		if err != nil {
			return nil, err
		}
		for _, catalog := range resp.Data.GetDiskResult.CatalogList {
			f := model.Object{
				ID:       catalog.CatalogID,
				Name:     catalog.CatalogName,
				Size:     0,
				Modified: getTime(catalog.UpdateTime),
				Ctime:    getTime(catalog.CreateTime),
				IsFolder: true,
			}
			files = append(files, &f)
		}
		for _, content := range resp.Data.GetDiskResult.ContentList {
			f := model.ObjThumb{
				Object: model.Object{
					ID:       content.ContentID,
					Name:     content.ContentName,
					Size:     content.ContentSize,
					Modified: getTime(content.UpdateTime),
					HashInfo: utils.NewHashInfo(utils.MD5, content.Digest),
				},
				Thumbnail: model.Thumbnail{Thumbnail: d.pickThumbnail(content.ThumbnailURL, content.BigthumbnailURL)},
			}
			files = append(files, &f)
		}
		if start+limit >= resp.Data.GetDiskResult.NodeCount {
			break
		}
		start += limit
	}
	return files, nil
}

// pickThumbnail returns the large thumbnail URL when UseLargeThumbnail is
// enabled and the API provided one, otherwise it falls back to the regular
// thumbnail URL.
func (d *Yun139) pickThumbnail(thumbnailURL, bigThumbnailURL string) string {
	if d.UseLargeThumbnail && bigThumbnailURL != "" {
		return bigThumbnailURL
	}
	return thumbnailURL
}

func (d *Yun139) newJson(data map[string]interface{}) base.Json {
	common := map[string]interface{}{
		"catalogType": 3,
		"cloudID":     d.CloudID,
		"cloudType":   1,
		"commonAccountInfo": base.Json{
			"account":     d.getAccount(),
			"accountType": 1,
		},
	}
	return utils.MergeMap(data, common)
}

func (d *Yun139) familyGetFiles(catalogID string) ([]model.Obj, error) {
	pageNum := 1
	files := make([]model.Obj, 0)
	for {
		data := d.newJson(base.Json{
			"catalogID":       catalogID,
			"contentSortType": 0,
			"pageInfo": base.Json{
				"pageNum":  pageNum,
				"pageSize": 100,
			},
			"sortDirection": 1,
		})
		var resp QueryContentListResp
		_, err := d.post("/orchestration/familyCloud-rebuild/content/v1.2/queryContentList", data, &resp)
		if err != nil {
			return nil, err
		}
		path := resp.Data.Path
		for _, catalog := range resp.Data.CloudCatalogList {
			f := model.Object{
				ID:       catalog.CatalogID,
				Name:     catalog.CatalogName,
				Size:     0,
				IsFolder: true,
				Modified: getTime(catalog.LastUpdateTime),
				Ctime:    getTime(catalog.CreateTime),
				Path:     path, // 文件夹上一级的Path
			}
			files = append(files, &f)
		}
		for _, content := range resp.Data.CloudContentList {
			f := model.ObjThumb{
				Object: model.Object{
					ID:       content.ContentID,
					Name:     content.ContentName,
					Size:     content.ContentSize,
					Modified: getTime(content.LastUpdateTime),
					Ctime:    getTime(content.CreateTime),
					Path:     path, // 文件所在目录的Path
				},
				Thumbnail: model.Thumbnail{Thumbnail: d.pickThumbnail(content.ThumbnailURL, content.BigthumbnailURL)},
			}
			files = append(files, &f)
		}
		if resp.Data.TotalCount == 0 {
			break
		}
		pageNum++
	}
	return files, nil
}

func (d *Yun139) groupGetFiles(catalogID string) ([]model.Obj, error) {
	pageNum := 1
	files := make([]model.Obj, 0)
	for {
		data := d.newJson(base.Json{
			"groupID":         d.CloudID,
			"catalogID":       path.Base(catalogID),
			"contentSortType": 0,
			"sortDirection":   1,
			"startNumber":     pageNum,
			"endNumber":       pageNum + 99,
			"path":            path.Join(d.RootFolderID, catalogID),
		})

		var resp QueryGroupContentListResp
		_, err := d.post("/orchestration/group-rebuild/content/v1.0/queryGroupContentList", data, &resp)
		if err != nil {
			return nil, err
		}
		path := resp.Data.GetGroupContentResult.ParentCatalogID
		for _, catalog := range resp.Data.GetGroupContentResult.CatalogList {
			f := model.Object{
				ID:       catalog.CatalogID,
				Name:     catalog.CatalogName,
				Size:     0,
				IsFolder: true,
				Modified: getTime(catalog.UpdateTime),
				Ctime:    getTime(catalog.CreateTime),
				Path:     catalog.Path, // 文件夹的真实Path， root:/开头
			}
			files = append(files, &f)
		}
		for _, content := range resp.Data.GetGroupContentResult.ContentList {
			f := model.ObjThumb{
				Object: model.Object{
					ID:       content.ContentID,
					Name:     content.ContentName,
					Size:     content.ContentSize,
					Modified: getTime(content.UpdateTime),
					Ctime:    getTime(content.CreateTime),
					Path:     path, // 文件所在目录的Path
				},
				Thumbnail: model.Thumbnail{Thumbnail: d.pickThumbnail(content.ThumbnailURL, content.BigthumbnailURL)},
			}
			files = append(files, &f)
		}
		if (pageNum + 99) > resp.Data.GetGroupContentResult.NodeCount {
			break
		}
		pageNum = pageNum + 100
	}
	return files, nil
}

func (d *Yun139) getLink(contentId string) (string, error) {
	data := base.Json{
		"appName":   "",
		"contentID": contentId,
		"commonAccountInfo": base.Json{
			"account":     d.getAccount(),
			"accountType": 1,
		},
	}
	res, err := d.post("/orchestration/personalCloud/uploadAndDownload/v1.0/downloadRequest",
		data, nil)
	if err != nil {
		return "", err
	}
	return jsoniter.Get(res, "data", "downloadURL").ToString(), nil
}
func (d *Yun139) familyGetLink(contentId string, path string) (string, error) {
	data := d.newJson(base.Json{
		"contentID": contentId,
		"path":      path,
	})
	res, err := d.post("/orchestration/familyCloud-rebuild/content/v1.0/getFileDownLoadURL",
		data, nil)
	if err != nil {
		return "", err
	}
	return jsoniter.Get(res, "data", "downloadURL").ToString(), nil
}

func (d *Yun139) groupGetLink(contentId string, path string) (string, error) {
	data := d.newJson(base.Json{
		"contentID": contentId,
		"groupID":   d.CloudID,
		"path":      path,
	})
	res, err := d.post("/orchestration/group-rebuild/groupManage/v1.0/getGroupFileDownLoadURL",
		data, nil)
	if err != nil {
		return "", err
	}
	return jsoniter.Get(res, "data", "downloadURL").ToString(), nil
}

func unicode(str string) string {
	textQuoted := strconv.QuoteToASCII(str)
	textUnquoted := textQuoted[1 : len(textQuoted)-1]
	return textUnquoted
}

func (d *Yun139) personalRequest(pathname string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	if err := d.ensurePersonalCloudHost(); err != nil {
		return nil, err
	}
	url := d.getPersonalCloudHost() + pathname
	req := base.RestyClient.R()
	randStr := random.String(16)
	ts := time.Now().Format("2006-01-02 15:04:05")
	if callback != nil {
		callback(req)
	}
	body, err := utils.Json.Marshal(req.Body)
	if err != nil {
		return nil, err
	}
	sign := calSign(string(body), ts, randStr)
	svcType := "1"
	if d.isFamily() {
		svcType = "2"
	}
	req.SetHeaders(map[string]string{
		"Accept":               "application/json, text/plain, */*",
		"Authorization":        "Basic " + d.getAuthorization(),
		"Caller":               "web",
		"Cms-Device":           "default",
		"Mcloud-Channel":       "1000101",
		"Mcloud-Client":        "10701",
		"Mcloud-Route":         "001",
		"Mcloud-Sign":          fmt.Sprintf("%s,%s,%s", ts, randStr, sign),
		"Mcloud-Version":       "7.14.0",
		"x-DeviceInfo":         "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||",
		"x-huawei-channelSrc":  "10000034",
		"x-inner-ntwk":         "2",
		"x-m4c-caller":         "PC",
		"x-m4c-src":            "10002",
		"x-SvcType":            svcType,
		"X-Yun-Api-Version":    "v1",
		"X-Yun-App-Channel":    "10000034",
		"X-Yun-Channel-Source": "10000034",
		"X-Yun-Client-Info":    "||9|7.14.0|chrome|120.0.0.0|||windows 10||zh-CN|||dW5kZWZpbmVk||",
		"X-Yun-Module-Type":    "100",
		"X-Yun-Svc-Type":       "1",
	})

	var e BaseResp
	req.SetResult(&e)
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	log.Debugln(res.String())
	if !e.Success {
		return nil, errors.New(e.Message)
	}
	if resp != nil {
		err = utils.Json.Unmarshal(res.Body(), resp)
		if err != nil {
			return nil, err
		}
	}
	return res.Body(), nil
}
func (d *Yun139) personalPost(pathname string, data interface{}, resp interface{}) ([]byte, error) {
	return d.personalRequest(pathname, http.MethodPost, func(req *resty.Request) {
		req.SetBody(data)
	}, resp)
}

func getPersonalTime(t string) time.Time {
	stamp, err := time.ParseInLocation("2006-01-02T15:04:05.999-07:00", t, utils.CNLoc)
	if err != nil {
		panic(err)
	}
	return stamp
}

func (d *Yun139) personalGetFiles(fileId string) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	nextPageCursor := ""
	for {
		data := base.Json{
			"imageThumbnailStyleList": []string{"Small", "Large"},
			"orderBy":                 "updated_at",
			"orderDirection":          "DESC",
			"pageInfo": base.Json{
				"pageCursor": nextPageCursor,
				"pageSize":   100,
			},
			"parentFileId": fileId,
		}
		var resp PersonalListResp
		_, err := d.personalPost("/file/list", data, &resp)
		if err != nil {
			return nil, err
		}
		nextPageCursor = resp.Data.NextPageCursor
		for _, item := range resp.Data.Items {
			var isFolder = (item.Type == "folder")
			var f model.Obj
			if isFolder {
				f = &model.Object{
					ID:       item.FileId,
					Name:     item.Name,
					Size:     0,
					Modified: getPersonalTime(item.UpdatedAt),
					Ctime:    getPersonalTime(item.CreatedAt),
					IsFolder: isFolder,
				}
			} else {
				var Thumbnails = item.Thumbnails
				var ThumbnailUrl string
				if d.UseLargeThumbnail {
					for _, thumb := range Thumbnails {
						if strings.Contains(thumb.Style, "Large") {
							ThumbnailUrl = thumb.Url
							break
						}
					}
				}
				if ThumbnailUrl == "" && len(Thumbnails) > 0 {
					ThumbnailUrl = Thumbnails[len(Thumbnails)-1].Url
				}
				f = &model.ObjThumb{
					Object: model.Object{
						ID:       item.FileId,
						Name:     item.Name,
						Size:     item.Size,
						Modified: getPersonalTime(item.UpdatedAt),
						Ctime:    getPersonalTime(item.CreatedAt),
						IsFolder: isFolder,
					},
					Thumbnail: model.Thumbnail{Thumbnail: ThumbnailUrl},
				}
			}
			files = append(files, f)
		}
		if len(nextPageCursor) == 0 {
			break
		}
	}
	return files, nil
}

func (d *Yun139) personalGetLink(fileId string) (string, error) {
	data := base.Json{
		"fileId": fileId,
	}
	res, err := d.personalPost("/file/getDownloadUrl",
		data, nil)
	if err != nil {
		return "", err
	}
	var cdnUrl = jsoniter.Get(res, "data", "cdnUrl").ToString()
	if cdnUrl != "" {
		return cdnUrl, nil
	} else {
		return jsoniter.Get(res, "data", "url").ToString(), nil
	}
}

func (d *Yun139) getAuthorization() string {
	if d.ref != nil {
		return d.ref.getAuthorization()
	}
	return d.Authorization
}
func (d *Yun139) getAccount() string {
	if d.ref != nil {
		return d.ref.getAccount()
	}
	return d.Account
}
func (d *Yun139) getPersonalCloudHost() string {
	if d.ref != nil {
		return d.ref.getPersonalCloudHost()
	}
	return d.PersonalCloudHost
}

func parseCookieMap(raw string) map[string]string {
	cookies := make(map[string]string)
	for _, c := range cookiepkg.Parse(raw) {
		if c.Name != "" {
			cookies[c.Name] = c.Value
		}
	}
	return cookies
}

func formatCookiesByOrder(cookies map[string]string, orderedNames []string, includeExtraNames bool) string {
	if len(cookies) == 0 {
		return ""
	}

	seen := make(map[string]struct{}, len(orderedNames))
	parts := make([]string, 0, len(cookies))
	for _, name := range orderedNames {
		seen[name] = struct{}{}
		if value, ok := cookies[name]; ok {
			parts = append(parts, name+"="+value)
		}
	}

	if includeExtraNames {
		extraNames := make([]string, 0, len(cookies))
		for name := range cookies {
			if _, ok := seen[name]; !ok {
				extraNames = append(extraNames, name)
			}
		}
		sort.Strings(extraNames)
		for _, name := range extraNames {
			parts = append(parts, name+"="+cookies[name])
		}
	}

	return strings.Join(parts, "; ")
}

func sanitizeLoginCookies(existingCookies string, newJSessionID string) string {
	cookies := parseCookieMap(existingCookies)
	delete(cookies, "JSESSIONID")
	if newJSessionID != "" {
		cookies["JSESSIONID"] = newJSessionID
	}
	return formatCookiesByOrder(cookies, mailLoginCookieOrder, false)
}

func mergeMailCookies(existingCookies string, responseCookies []*http.Cookie) string {
	cookies := parseCookieMap(existingCookies)
	for _, c := range responseCookies {
		if c.Name != "" {
			cookies[c.Name] = c.Value
		}
	}
	return formatCookiesByOrder(cookies, mailLoginCookieOrder, true)
}

func extractFastLoginCookies(mailCookies string) (sid string, rmkey string) {
	for _, c := range cookiepkg.Parse(mailCookies) {
		switch c.Name {
		case "Os_SSo_Sid":
			sid = c.Value
		case "RMKEY":
			rmkey = c.Value
		}
		if sid != "" && rmkey != "" {
			return sid, rmkey
		}
	}
	return sid, rmkey
}

func isRedirectStatus(statusCode int) bool {
	return statusCode >= 300 && statusCode <= 399
}

func hasCookiePair(raw string) bool {
	for _, part := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.TrimSpace(name) != "" && value != "" {
			return true
		}
	}
	return false
}

func (d *Yun139) step1_password_login() (string, error) {
	log.Debugf("--- 执行步骤 1: 登录 API ---")
	loginURL := "https://mail.10086.cn/Login/Login.ashx"

	getResp, err := base.RestyClient.R().Get(loginURL)
	if err != nil {
		return "", fmt.Errorf("step1 get jsessionid failed: %w", err)
	}
	var jsessionid string
	for _, cookie := range getResp.Cookies() {
		if cookie.Name == "JSESSIONID" {
			jsessionid = cookie.Value
			break
		}
	}
	if jsessionid == "" {
		log.Warnf("139yun: failed to get JSESSIONID from GET request.")
	}

	hashedPassword := sha1Hash(fmt.Sprintf("fetion.com.cn:%s", d.Password))
	cguid := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sanitizedCookie := sanitizeLoginCookies(d.MailCookies, jsessionid)

	loginHeaders := map[string]string{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"accept-language":           "zh-CN,zh;q=0.9,zh-TW;q=0.8,en-US;q=0.7,en;q=0.6,en-GB;q=0.5",
		"cache-control":             "max-age=0",
		"content-type":              "application/x-www-form-urlencoded",
		"dnt":                       "1",
		"origin":                    "https://mail.10086.cn",
		"priority":                  "u=0, i",
		"referer":                   fmt.Sprintf("https://mail.10086.cn/default.html?&s=1&v=0&u=%s&m=1&ec=S001&resource=indexLogin&clientid=1003&auto=on&cguid=%s&mtime=45", base64.StdEncoding.EncodeToString([]byte(d.Username)), cguid),
		"sec-ch-ua":                 "\"Microsoft Edge\";v=\"141\", \"Not?A_Brand\";v=\"8\", \"Chromium\";v=\"141\"",
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        "\"Windows\"",
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "same-origin",
		"sec-fetch-user":            "?1",
		"upgrade-insecure-requests": "1",
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36 Edg/141.0.0.0",
		"Cookie":                    sanitizedCookie,
	}

	loginData := url.Values{}
	loginData.Set("UserName", d.Username)
	loginData.Set("passOld", "")
	loginData.Set("auto", "on")
	loginData.Set("Password", hashedPassword)
	loginData.Set("webIndexPagePwdLogin", "1")
	loginData.Set("pwdType", "1")
	loginData.Set("clientId", "1003")
	loginData.Set("authType", "2")

	log.Debugf("DEBUG: 登录请求已准备，cookie_count=%d", len(cookiepkg.Parse(sanitizedCookie)))

	client := resty.New().SetRedirectPolicy(resty.NoRedirectPolicy())
	res, err := client.R().
		SetHeaders(loginHeaders).
		SetFormDataFromValues(loginData).
		Post(loginURL)

	if res == nil {
		return "", fmt.Errorf("step1 login request failed: response is nil (error: %v)", err)
	}
	if err != nil && !isRedirectStatus(res.StatusCode()) {
		return "", fmt.Errorf("step1 login request failed: status %d: %w", res.StatusCode(), err)
	}
	log.Debugf("DEBUG: 登录响应 Status Code: %d", res.StatusCode())
	log.Debugf("DEBUG: 登录响应 Location present: %t", res.Header().Get("Location") != "")

	var sid, extractedCguid string
	locationHeader := res.Header().Get("Location")
	if locationHeader != "" {
		if ecMatch := regexp.MustCompile(`ec=([^&]+)`).FindStringSubmatch(locationHeader); len(ecMatch) > 1 {
			return "", fmt.Errorf("risk control triggered: %s", ecMatch[0])
		}

		sidMatch := regexp.MustCompile(`sid=([^&]+)`).FindStringSubmatch(locationHeader)
		cguidMatch := regexp.MustCompile(`cguid=([^&]+)`).FindStringSubmatch(locationHeader)

		if len(sidMatch) > 1 {
			sid = sidMatch[1]
			log.Debugf("DEBUG: 从 Location 提取到 sid.")
		} else if strings.Contains(locationHeader, "default.html") {
			return "", errors.New("authentication failed: sid is missing in default.html redirect")
		}

		if len(cguidMatch) > 1 {
			extractedCguid = cguidMatch[1]
			log.Debugf("DEBUG: 从 Location 提取到 cguid.")
		}
	}

	if sid == "" || extractedCguid == "" {
		for _, cookieStr := range res.Header().Values("Set-Cookie") {
			ssoSidMatch := regexp.MustCompile(`Os_SSo_Sid=([^;]+)`).FindStringSubmatch(cookieStr)
			cookieCguidMatch := regexp.MustCompile(`cguid=([^;]+)`).FindStringSubmatch(cookieStr)
			if len(ssoSidMatch) > 1 && sid == "" {
				sid = ssoSidMatch[1]
				log.Debugf("DEBUG: 从 Set-Cookie 提取到 sid.")
			}
			if len(cookieCguidMatch) > 1 && extractedCguid == "" {
				extractedCguid = cookieCguidMatch[1]
				log.Debugf("DEBUG: 从 Set-Cookie 提取到 cguid.")
			}
		}
	}

	if sid == "" || extractedCguid == "" {
		return "", errors.New("failed to extract sid or cguid from login response")
	}

	d.MailCookies = mergeMailCookies(d.MailCookies, res.Cookies())
	log.Debugf("DEBUG: 更新后的 Cookies 数量: %d", len(cookiepkg.Parse(d.MailCookies)))

	return sid, nil
}

func (d *Yun139) step2_get_single_token(sid string) (string, error) {
	log.Debugf("\n--- 执行步骤 2: 换artifact API ---")
	cguid := strconv.FormatInt(time.Now().UnixMilli(), 10)
	exchangeArtifactURL := fmt.Sprintf("https://smsrebuild1.mail.10086.cn/setting/s?func=%s&sid=%s&cguid=%s", url.QueryEscape("umc:getArtifact"), sid, cguid)

	_, rmkey := extractFastLoginCookies(d.MailCookies)
	if rmkey == "" {
		return "", errors.New("RMKEY not found in MailCookies")
	}

	res, err := base.RestyClient.R().
		SetHeaders(map[string]string{
			"Host":            "smsrebuild1.mail.10086.cn",
			"Cookie":          "RMKEY=" + rmkey,
			"Content-Type":    "text/xml; charset=utf-8",
			"Accept-Encoding": "gzip",
			"User-Agent":      "okhttp/4.12.0",
		}).
		Post(exchangeArtifactURL)
	if err != nil {
		return "", fmt.Errorf("step2 exchange artifact request failed: %w", err)
	}

	log.Debugf("DEBUG: 换passid 响应 Status Code: %d", res.StatusCode())
	log.Debugf("DEBUG: 换passid 响应 Body length: %d", len(res.Body()))

	dycpwd := jsoniter.Get(res.Body(), "var", "artifact").ToString()
	if dycpwd == "" {
		code := jsoniter.Get(res.Body(), "code").ToString()
		summary := jsoniter.Get(res.Body(), "summary").ToString()
		if code == "" {
			if match := regexp.MustCompile(`['"]code['"]\s*:\s*['"]([^'"]+)['"]`).FindSubmatch(res.Body()); len(match) == 2 {
				code = string(match[1])
			}
		}
		if summary == "" {
			if match := regexp.MustCompile(`['"]summary['"]\s*:\s*['"]([^'"]+)['"]`).FindSubmatch(res.Body()); len(match) == 2 {
				summary = string(match[1])
			}
		}
		if code != "" || summary != "" {
			return "", fmt.Errorf("failed to extract dycpwd from artifact exchange response: code=%s summary=%s", code, summary)
		}
		return "", errors.New("failed to extract dycpwd from artifact exchange response")
	}
	log.Debugf("DEBUG: dycpwd extracted from artifact exchange response.")
	return dycpwd, nil
}

func sha1Hash(data string) string {
	h := sha1.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("pkcs7: data is empty")
	}
	unpadding := int(data[length-1])
	if unpadding > length {
		return nil, errors.New("pkcs7: invalid padding")
	}
	return data[:length-unpadding], nil
}

func aesEcbDecrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("AES ECB decrypt: ciphertext is not a multiple of the block size")
	}

	decrypted := make([]byte, len(ciphertext))
	blockSize := block.BlockSize()
	for bs, be := 0, blockSize; bs < len(ciphertext); bs, be = bs+blockSize, be+blockSize {
		block.Decrypt(decrypted[bs:be], ciphertext[bs:be])
	}
	return pkcs7Unpad(decrypted)
}

func aesCbcEncrypt(plaintext []byte, key []byte, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("aesCbcEncrypt: iv length %d does not match block size %d", len(iv), block.BlockSize())
	}
	padded := pkcs7Pad(plaintext, block.BlockSize())
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)
	return ciphertext, nil
}

func aesCbcDecrypt(ciphertext []byte, key []byte, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("aesCbcDecrypt: iv length %d does not match block size %d", len(iv), block.BlockSize())
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("aesCbcDecrypt: ciphertext is not a multiple of the block size")
	}
	decrypted := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, ciphertext)
	return pkcs7Unpad(decrypted)
}

func sortedJsonStringify(obj interface{}) (string, error) {
	if obj == nil {
		return "null", nil
	}

	switch v := obj.(type) {
	case string:
		var parsed interface{}
		if err := jsoniter.Unmarshal([]byte(v), &parsed); err == nil {
			return sortedJsonStringify(parsed)
		}
		return jsoniter.MarshalToString(v)
	case int, float64, bool:
		return fmt.Sprintf("%v", v), nil
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			s, err := sortedJsonStringify(item)
			if err != nil {
				return "", err
			}
			items = append(items, s)
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ",")), nil
	case map[string]interface{}:
		sortedKeys := make([]string, 0, len(v))
		for key := range v {
			sortedKeys = append(sortedKeys, key)
		}
		sort.Strings(sortedKeys)

		pairs := make([]string, 0, len(v))
		for _, key := range sortedKeys {
			value, err := sortedJsonStringify(v[key])
			if err != nil {
				return "", err
			}
			keyStr, err := jsoniter.MarshalToString(key)
			if err != nil {
				return "", err
			}
			pairs = append(pairs, fmt.Sprintf("%s:%s", keyStr, value))
		}
		return fmt.Sprintf("{%s}", strings.Join(pairs, ",")), nil
	default:
		return jsoniter.MarshalToString(v)
	}
}

func (d *Yun139) yun139EncryptedRequest(url string, body interface{}, headers map[string]string, aesKeyHex string, resp interface{}) ([]byte, error) {
	aesKey, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		return nil, fmt.Errorf("yun139EncryptedRequest: failed to decode AES key: %w", err)
	}

	sortedJson, err := sortedJsonStringify(body)
	if err != nil {
		return nil, fmt.Errorf("yun139EncryptedRequest: failed to marshal and sort body: %w", err)
	}
	log.Debugf("yun139EncryptedRequest: plaintext request body prepared, length=%d", len(sortedJson))

	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("yun139EncryptedRequest: failed to generate IV: %w", err)
	}
	encryptedBody, err := aesCbcEncrypt([]byte(sortedJson), aesKey, iv)
	if err != nil {
		return nil, fmt.Errorf("yun139EncryptedRequest: failed to encrypt body: %w", err)
	}
	payload := base64.StdEncoding.EncodeToString(append(iv, encryptedBody...))

	res, err := base.RestyClient.R().
		SetHeaders(headers).
		SetBody(payload).
		Post(url)
	if err != nil {
		return nil, fmt.Errorf("yun139EncryptedRequest: http request failed: %w", err)
	}
	if res.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("yun139EncryptedRequest: unexpected status code %d: %s", res.StatusCode(), res.String())
	}

	respBody := res.Body()
	var decryptedBytes []byte
	if len(respBody) > 0 && respBody[0] == '{' {
		log.Warnf("yun139EncryptedRequest: received a plain JSON response, not an encrypted string, length=%d", len(respBody))
		decryptedBytes = respBody
	} else {
		decodedResp, err := base64.StdEncoding.DecodeString(string(respBody))
		if err != nil {
			return nil, fmt.Errorf("yun139EncryptedRequest: response base64 decode failed: %w. Body: '%s'", err, string(respBody))
		}
		if len(decodedResp) < 16 {
			return nil, fmt.Errorf("yun139EncryptedRequest: decoded response is too short to be encrypted. Length: %d", len(decodedResp))
		}
		decryptedBytes, err = aesCbcDecrypt(decodedResp[16:], aesKey, decodedResp[:16])
		if err != nil {
			return nil, fmt.Errorf("yun139EncryptedRequest: response aes decrypt failed: %w", err)
		}
	}
	log.Debugf("yun139EncryptedRequest: decrypted response body received, length=%d", len(decryptedBytes))

	if resp != nil {
		if err := utils.Json.Unmarshal(decryptedBytes, resp); err != nil {
			return nil, fmt.Errorf("yun139EncryptedRequest: failed to unmarshal decrypted response: %w", err)
		}
	}
	return decryptedBytes, nil
}

func (d *Yun139) step3_third_party_login(dycpwd string) (string, error) {
	log.Debugf("\n--- 执行步骤 3: 单点登录 API ---")
	ssoLoginURL := "https://user-njs.yun.139.com/user/thirdlogin"

	decryptedLayer1StrBytes, err := d.yun139EncryptedRequest(ssoLoginURL, base.Json{
		"clientkey_decrypt": "l3TryM&Q+X7@dzwk)qP",
		"clienttype":        "886",
		"cpid":              "507",
		"dycpwd":            dycpwd,
		"extInfo":           base.Json{"ifOpenAccount": "0"},
		"loginMode":         "0",
		"msisdn":            d.Username,
		"pintype":           "13",
		"secinfo":           strings.ToUpper(sha1Hash(fmt.Sprintf("fetion.com.cn:%s", dycpwd))),
		"version":           "20250901",
	}, map[string]string{
		"hcy-cool-flag":       "1",
		"x-huawei-channelSrc": "10246600",
		"x-sdk-channelSrc":    "",
		"x-MM-Source":         "0",
		"x-UserAgent":         "android|23116PN5BC|android15|1.2.6|||1440x3200|10246600",
		"x-DeviceInfo":        "4|127.0.0.1|5|1.2.6|Xiaomi|23116PN5BC||02-00-00-00-00-00|android 15|1440x3200|android|||",
		"Content-Type":        "text/plain;charset=UTF-8",
		"Host":                "user-njs.yun.139.com",
		"Accept-Encoding":     "gzip",
		"User-Agent":          "okhttp/3.12.2",
	}, KEY_HEX_1, nil)
	if err != nil {
		return "", fmt.Errorf("step3 encrypted request failed: %w", err)
	}

	hexInner := jsoniter.Get(decryptedLayer1StrBytes, "data").ToString()
	if hexInner == "" {
		return "", errors.New("missing data field in first layer decryption result")
	}
	log.Debugf("DEBUG: 第一层解密提取到 hex_inner, length=%d", len(hexInner))

	key2, err := hex.DecodeString(KEY_HEX_2)
	if err != nil {
		return "", fmt.Errorf("failed to decode KEY_HEX_2: %w", err)
	}
	hexInnerBytes, err := hex.DecodeString(hexInner)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex_inner: %w", err)
	}
	finalJsonStrBytes, err := aesEcbDecrypt(hexInnerBytes, key2)
	if err != nil {
		return "", fmt.Errorf("step3 response layer2 aes ecb decrypt failed: %w", err)
	}
	log.Debugf("DEBUG: third party login response decrypted.")

	authToken := jsoniter.Get(finalJsonStrBytes, "authToken").ToString()
	if authToken == "" {
		return "", errors.New("failed to extract authToken from final decryption result")
	}

	account := jsoniter.Get(finalJsonStrBytes, "account").ToString()
	userDomainId := jsoniter.Get(finalJsonStrBytes, "userDomainId").ToString()
	if account == "" || userDomainId == "" {
		return "", errors.New("failed to extract account or userDomainId from final decryption result")
	}

	d.Account = account
	d.UserDomainID = userDomainId
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("pc:%s:%s", account, authToken))), nil
}

func (d *Yun139) validateAndInitCredentials() error {
	state, err := d.credentialState()
	if err != nil {
		return err
	}

	switch state {
	case credentialStateAuthorization:
		log.Debugf("139yun: Authorization exists, skipping initialization login.")
		return nil
	case credentialStateFullLogin, credentialStateCookiesOnly:
		log.Infof("139yun: Authorization missing, attempting login...")
		if d.tryFastLoginWithCookies() {
			return nil
		}
		if state == credentialStateCookiesOnly {
			return fmt.Errorf("fast login with cookies failed, and cannot fallback to password login (missing username/password)")
		}
		log.Infof("139yun: fast login failed or not possible, performing full password login (Step 1).")
		_, err := d.loginWithPassword()
		if err != nil {
			return fmt.Errorf("login with password failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported credential state: %d", state)
	}
}

func (d *Yun139) credentialState() (credentialState, error) {
	d.Authorization = strings.TrimSpace(d.Authorization)
	d.Username = strings.TrimSpace(d.Username)
	d.MailCookies = strings.TrimSpace(d.MailCookies)

	if d.Authorization != "" {
		if strings.HasPrefix(strings.ToLower(d.Authorization), "basic ") {
			return 0, fmt.Errorf("authorization should not include Basic prefix")
		}
		return credentialStateAuthorization, nil
	}

	if d.MailCookies != "" && !hasCookiePair(d.MailCookies) {
		return 0, fmt.Errorf("MailCookies format is invalid, please check your configuration")
	}

	hasUsername := d.Username != ""
	hasPassword := strings.TrimSpace(d.Password) != ""
	hasCookies := d.MailCookies != ""
	if hasUsername || hasPassword {
		if !hasUsername || !hasPassword || !hasCookies {
			return 0, fmt.Errorf("if username or password is provided, all three (mail_cookies, username, password) must be provided")
		}
		return credentialStateFullLogin, nil
	}

	if hasCookies {
		return credentialStateCookiesOnly, nil
	}

	return 0, fmt.Errorf("authorization is empty and credentials are not provided")
}

func (d *Yun139) tryFastLoginWithCookies() bool {
	sid, rmkey := extractFastLoginCookies(d.MailCookies)
	if sid == "" || rmkey == "" {
		log.Warnf("139yun: fast login skipped, required cookies missing: Os_SSo_Sid=%t RMKEY=%t", sid != "", rmkey != "")
		return false
	}

	log.Infof("139yun: attempting fast login using existing SID/Cookies (Step 2).")
	token, err := d.step2_get_single_token(sid)
	if err != nil || token == "" {
		log.Warnf("139yun: fast login Step 2 failed: %v", err)
		return false
	}

	log.Infof("139yun: Step 2 success. Proceeding to Step 3.")
	auth, err := d.step3_third_party_login(token)
	if err != nil {
		log.Warnf("139yun: fast login Step 3 failed: %v", err)
		return false
	}

	d.Authorization = auth
	op.MustSaveDriverStorage(d)
	log.Infof("139yun: fast login success (Step 2 -> Step 3).")
	return true
}

func (d *Yun139) loginWithPassword() (string, error) {
	if d.Username == "" || d.Password == "" || d.MailCookies == "" {
		return "", errors.New("username, password or mail_cookies is empty")
	}

	passId, err := d.step1_password_login()
	if err != nil {
		return "", err
	}
	log.Infof("Step 1 success.")

	token, err := d.step2_get_single_token(passId)
	if err != nil {
		return "", err
	}
	log.Infof("Step 2 success.")

	newAuth, err := d.step3_third_party_login(token)
	if err != nil {
		return "", err
	}
	log.Infof("Step 3 success, new authorization generated.")

	d.Authorization = newAuth
	op.MustSaveDriverStorage(d)
	return newAuth, nil
}

func (d *Yun139) sharePost(pathname string, data interface{}, resp interface{}) ([]byte, error) {
	crypto := NewYunCrypto()
	encryptedBody, err := crypto.Encrypt(data)
	if err != nil {
		return nil, err
	}

	url := "https://share-kd-njs.yun.139.com" + pathname
	req := base.RestyClient.R()

	auth := d.getAuthorization()
	if !strings.HasPrefix(auth, "Basic ") {
		auth = "Basic " + auth
	}
	// randStr := random.String(16)
	// ts := time.Now().Format("2006-01-02 15:04:05")
	// body, err := utils.Json.Marshal(req.Body)
	// if err != nil {
	// 	return nil, err
	// }
	// sign := calSign(string(body), ts, randStr)
	// svcType := "1"
	// if d.isFamily() {
	// 	svcType = "2"
	// }
	req.SetHeaders(map[string]string{
		"User-Agent":        "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0",
		"Accept":            "application/json, text/plain, */*",
		"Content-Type":      "application/json;charset=UTF-8",
		"Authorization":     auth,
		"X-Deviceinfo":      "||9|12.27.0|firefox|140.0|12b780037221ab547c682223327dc9cd||linux unknow|1920X526|zh-CN|||",
		"hcy-cool-flag":     "1",
		"CMS-DEVICE":        "default",
		"x-m4c-caller":      "PC",
		"X-Yun-Api-Version": "v1",
		"Origin":            "https://yun.139.com",
		"Referer":           "https://yun.139.com/",
	})
	req.SetBody(encryptedBody)

	res, err := req.Post(url)
	if err != nil {
		return nil, err
	}

	decryptedText, err := crypto.Decrypt(res.String())
	if err != nil {
		log.Errorf("[139Share] Decryption failed, raw response: %s", res.String())
		return nil, fmt.Errorf("decryption failed: %v, raw: %s", err, res.String())
	}

	if resp != nil {
		err = utils.Json.Unmarshal([]byte(decryptedText), resp)
		if err != nil {
			return nil, err
		}
	}
	return []byte(decryptedText), nil
}

func (d *Yun139) shareGetFiles(pCaID string) ([]model.Obj, error) {
	if pCaID == "" {
		pCaID = "root"
	}
	data := base.Json{
		"getOutLinkInfoReq": base.Json{
			"account": d.getAccount(),
			"linkID":  d.LinkID,
			"pCaID":   pCaID,
		},
	}
	var resp ShareListResp
	_, err := d.sharePost("/yun-share/richlifeApp/devapp/IOutLink/getOutLinkInfoV6", data, &resp)
	if err != nil {
		return nil, err
	}
	files := make([]model.Obj, 0)
	// 直接从 Data 中读取 CaLst
	for _, catalog := range resp.Data.CaLst {
		modTime, _ := time.ParseInLocation("20060102150405", catalog.UdTime, utils.CNLoc)
		f := model.Object{
			ID:       catalog.CaID,
			Name:     catalog.CaName,
			Modified: modTime,
			IsFolder: true,
		}
		files = append(files, &f)
	}
	for _, content := range resp.Data.CoLst {
		name := content.CoName
		size := content.CoSize
		// Force .m3u8 suffix for videos and declare 1MB size for padding logic
		if content.CoType == 3 || strings.HasSuffix(strings.ToLower(name), ".mp4") {
			if !strings.HasSuffix(name, ".m3u8") {
				name += ".m3u8"
			}
			size = 1024 * 1024 // Key: declare 1MB to match RangeReadCloser padding
		}
		modTime, _ := time.ParseInLocation("20060102150405", content.UdTime, utils.CNLoc)
		f := model.Object{
			ID:       content.CoID,
			Name:     name,
			Size:     size,
			Modified: modTime,
		}
		files = append(files, &f)
	}

	return files, nil
}

type YunCrypto struct {
	Key       []byte
	BlockSize int
}

func NewYunCrypto() *YunCrypto {
	return &YunCrypto{
		Key:       []byte("PVGDwmcvfs1uV3d1"),
		BlockSize: aes.BlockSize,
	}
}

func (y *YunCrypto) PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func (y *YunCrypto) PKCS7UnPadding(origData []byte) ([]byte, error) {
	length := len(origData)
	if length == 0 {
		return nil, errors.New("data is empty")
	}
	unpadding := int(origData[length-1])
	if length < unpadding {
		return nil, errors.New("unpadding error")
	}
	return origData[:(length - unpadding)], nil
}

func (y *YunCrypto) Encrypt(data interface{}) (string, error) {
	jsonData, err := utils.Json.Marshal(data)
	if err != nil {
		return "", err
	}
	iv := make([]byte, y.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(y.Key)
	if err != nil {
		return "", err
	}
	content := y.PKCS7Padding(jsonData, y.BlockSize)
	ciphertext := make([]byte, len(content))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, content)
	result := append(iv, ciphertext...)
	return base64.StdEncoding.EncodeToString(result), nil
}

func (y *YunCrypto) Decrypt(b64Data string) (string, error) {
	b64Data = strings.Join(strings.Fields(b64Data), "")
	raw, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return "", err
	}
	if len(raw) < y.BlockSize {
		return "", errors.New("data too short")
	}
	iv := raw[:y.BlockSize]
	ciphertext := raw[y.BlockSize:]
	block, err := aes.NewCipher(y.Key)
	if err != nil {
		return "", err
	}
	decrypted := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, ciphertext)
	if len(decrypted) > 2 && decrypted[0] == 0x1f && decrypted[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(decrypted))
		if err == nil {
			defer reader.Close()
			unzipped, err := io.ReadAll(reader)
			if err == nil {
				return string(unzipped), nil
			}
		}
	}
	unpadded, err := y.PKCS7UnPadding(decrypted)
	if err != nil {
		return strings.TrimSpace(string(decrypted)), nil
	}
	return string(unpadded), nil
}

func (d *Yun139) rewriteM3U8(masterURL string) (string, error) {
	client := resty.New().SetTimeout(10 * time.Second)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		"Referer":    "https://yun.139.com/",
	}

	// 1. Get Master M3U8
	resp, err := client.R().SetHeaders(headers).Get(masterURL)
	if err != nil {
		return "", err
	}
	masterContent := resp.String()

	// 2. Find sub-playlist path
	var subRelPath string
	lines := strings.Split(masterContent, "\n")
	for i, line := range lines {
		if strings.Contains(line, "RESOLUTION=") {
			if i+1 < len(lines) {
				subRelPath = strings.TrimSpace(lines[i+1])
				if strings.Contains(line, "1920x1080") {
					break
				}
			}
		}
	}
	if subRelPath == "" {
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" && !strings.HasPrefix(line, "#") {
				subRelPath = line
				break
			}
		}
	}
	if subRelPath == "" {
		return "", fmt.Errorf("sub playlist not found in master m3u8")
	}

	// 3. Get sub-playlist content
	base, _ := url.Parse(masterURL)
	ref, _ := url.Parse(subRelPath)
	subURL := base.ResolveReference(ref).String()

	resp, err = client.R().SetHeaders(headers).Get(subURL)
	if err != nil {
		return "", err
	}
	subContent := resp.String()

	// 4. Resolve relative TS paths to absolute URLs
	subBase, _ := url.Parse(subURL)
	subLines := strings.Split(subContent, "\n")
	var finalLines []string
	for _, line := range subLines {
		cleanLine := strings.TrimSpace(line)
		if cleanLine != "" && !strings.HasPrefix(cleanLine, "#") {
			if !strings.HasPrefix(cleanLine, "http") {
				tsRef, _ := url.Parse(cleanLine)
				finalLines = append(finalLines, subBase.ResolveReference(tsRef).String())
			} else {
				finalLines = append(finalLines, cleanLine)
			}
		} else {
			finalLines = append(finalLines, line)
		}
	}

	finalM3U8 := strings.Join(finalLines, "\n")

	return finalM3U8, nil
}

func (d *Yun139) Proxy(c *gin.Context, obj model.Obj) error {
	return nil
}

func (d *Yun139) shareGetLink(coID string) (*model.Link, error) {
	data := base.Json{
		"getContentInfoFromOutLinkReq": base.Json{
			"contentId": coID,
			"linkID":    d.LinkID,
			"account":   d.getAccount(),
		},
	}
	var resp ShareContentInfoResp
	_, err := d.sharePost("/yun-share/richlifeApp/devapp/IOutLink/getContentInfoFromOutLink", data, &resp)
	if err != nil {
		return nil, err
	}

	res := resp.Data.ContentInfo
	if res.PresentURL != "" {
		m3u8Content, err := d.rewriteM3U8(res.PresentURL)
		if err != nil {
			return nil, err
		}

		// Core logic: pad to 1MB to ensure compatibility with AList's size validation
		targetSize := int64(1024 * 1024)
		contentBytes := []byte(m3u8Content)
		if int64(len(contentBytes)) < targetSize {
			padding := bytes.Repeat([]byte(" "), int(targetSize-int64(len(contentBytes))))
			contentBytes = append(contentBytes, padding...)
		} else {
			// Truncate if M3U8 exceeds 1MB (extremely rare)
			contentBytes = contentBytes[:targetSize]
		}

		return &model.Link{
			RangeReadCloser: &model.RangeReadCloser{
				RangeReader: func(ctx context.Context, range_ http_range.Range) (io.ReadCloser, error) {
					reader := bytes.NewReader(contentBytes)
					// Handle AList Range requests
					_, _ = reader.Seek(range_.Start, io.SeekStart)
					// Wrap as ReadCloser
					return io.NopCloser(reader), nil
				},
			},
			Header: http.Header{
				"Content-Type": []string{"application/vnd.apple.mpegurl"},
			},
		}, nil
	}

	if res.DownloadURL != "" {
		return &model.Link{URL: res.DownloadURL}, nil
	}

	return nil, fmt.Errorf("failed to get link")
}
