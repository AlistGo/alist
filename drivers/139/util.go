package _139

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/pkg/http_range"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
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
	if err != nil {
		return err
	}
	if resp.Return != "0" {
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
				Thumbnail: model.Thumbnail{Thumbnail: content.ThumbnailURL},
				//Thumbnail: content.BigthumbnailURL,
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
				Thumbnail: model.Thumbnail{Thumbnail: content.ThumbnailURL},
				//Thumbnail: content.BigthumbnailURL,
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
				Thumbnail: model.Thumbnail{Thumbnail: content.ThumbnailURL},
				//Thumbnail: content.BigthumbnailURL,
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
