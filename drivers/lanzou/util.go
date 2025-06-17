package lanzou

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

var upClient *resty.Client
var once sync.Once

func (d *LanZou) doupload(callback base.ReqCallback, resp interface{}) ([]byte, error) {
	return d.post(d.BaseUrl+"/doupload.php", func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"uid": d.uid,
			"vei": d.vei,
		})
		if callback != nil {
			callback(req)
		}
	}, resp)
}

func (d *LanZou) get(url string, callback base.ReqCallback) ([]byte, error) {
	return d.request(url, http.MethodGet, callback, false)
}

func (d *LanZou) post(url string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	data, err := d._post(url, callback, resp, false)
	if err == ErrCookieExpiration && d.IsAccount() {
		if atomic.CompareAndSwapInt32(&d.flag, 0, 1) {
			_, err2 := d.Login()
			atomic.SwapInt32(&d.flag, 0)
			if err2 != nil {
				err = errors.Join(err, err2)
				d.Status = err.Error()
				op.MustSaveDriverStorage(d)
				return data, err
			}
		}
		for atomic.LoadInt32(&d.flag) != 0 {
			runtime.Gosched()
		}
		return d._post(url, callback, resp, false)
	}
	return data, err
}

func (d *LanZou) _post(url string, callback base.ReqCallback, resp interface{}, up bool) ([]byte, error) {
	data, err := d.request(url, http.MethodPost, func(req *resty.Request) {
		req.AddRetryCondition(func(r *resty.Response, err error) bool {
			if utils.Json.Get(r.Body(), "zt").ToInt() == 4 {
				time.Sleep(time.Second)
				return true
			}
			return false
		})
		if callback != nil {
			callback(req)
		}
	}, up)
	if err != nil {
		return data, err
	}
	switch utils.Json.Get(data, "zt").ToInt() {
	case 1, 2, 4:
		if resp != nil {
			// 返回类型不统一,忽略错误
			utils.Json.Unmarshal(data, resp)
		}
		return data, nil
	case 9: // 登录过期
		return data, ErrCookieExpiration
	default:
		info := utils.Json.Get(data, "inf").ToString()
		if info == "" {
			info = utils.Json.Get(data, "info").ToString()
		}
		return data, fmt.Errorf(info)
	}
}

func (d *LanZou) request(url string, method string, callback base.ReqCallback, up bool) ([]byte, error) {
	var req *resty.Request
	if up {
		once.Do(func() {
			upClient = base.NewRestyClient().SetTimeout(120 * time.Second)
		})
		req = upClient.R()
	} else {
		req = base.RestyClient.R()
	}

	req.SetHeaders(map[string]string{
		"Referer":    "https://pc.woozooo.com",
		"User-Agent": d.UserAgent,
	})

	if d.Cookie != "" {
		req.SetHeader("cookie", d.Cookie)
	}

	if callback != nil {
		callback(req)
	}

	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	log.Debugf("lanzou request: url=>%s ,stats=>%d ,body => %s\n", res.Request.URL, res.StatusCode(), res.String())
	return res.Body(), err
}

func (d *LanZou) Login() ([]*http.Cookie, error) {
	resp, err := base.NewRestyClient().SetRedirectPolicy(resty.NoRedirectPolicy()).
		R().SetFormData(map[string]string{
		"task":         "3",
		"uid":          d.Account,
		"pwd":          d.Password,
		"setSessionId": "",
		"setSig":       "",
		"setScene":     "",
		"setTocen":     "",
		"formhash":     "",
	}).Post("https://up.woozooo.com/mlogin.php")
	if err != nil {
		return nil, err
	}
	if utils.Json.Get(resp.Body(), "zt").ToInt() != 1 {
		return nil, fmt.Errorf("login err: %s", resp.Body())
	}
	d.Cookie = CookieToString(resp.Cookies())
	return resp.Cookies(), nil
}

/*
通过cookie获取数据
*/

// 获取文件和文件夹,获取到的文件大小、更改时间不可信
func (d *LanZou) GetAllFiles(folderID string) ([]model.Obj, error) {
	folders, err := d.GetFolders(folderID)
	if err != nil {
		return nil, err
	}
	files, err := d.GetFiles(folderID)
	if err != nil {
		return nil, err
	}
	return append(
		utils.MustSliceConvert(folders, func(folder FileOrFolder) model.Obj {
			return &folder
		}), utils.MustSliceConvert(files, func(file FileOrFolder) model.Obj {
			return &file
		})...,
	), nil
}

// 通过ID获取文件夹
func (d *LanZou) GetFolders(folderID string) ([]FileOrFolder, error) {
	var resp RespText[[]FileOrFolder]
	_, err := d.doupload(func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"task":      "47",
			"folder_id": folderID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Text, nil
}

// 通过ID获取文件
func (d *LanZou) GetFiles(folderID string) ([]FileOrFolder, error) {
	files := make([]FileOrFolder, 0)
	for pg := 1; ; pg++ {
		var resp RespText[[]FileOrFolder]
		_, err := d.doupload(func(req *resty.Request) {
			req.SetFormData(map[string]string{
				"task":      "5",
				"folder_id": folderID,
				"pg":        strconv.Itoa(pg),
			})
		}, &resp)
		if err != nil {
			return nil, err
		}
		if len(resp.Text) == 0 {
			break
		}
		files = append(files, resp.Text...)
	}
	return files, nil
}

// 通过ID获取文件夹分享地址
func (d *LanZou) getFolderShareUrlByID(fileID string) (*FileShare, error) {
	var resp RespInfo[FileShare]
	_, err := d.doupload(func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"task":    "18",
			"file_id": fileID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Info, nil
}

// 通过ID获取文件分享地址
func (d *LanZou) getFileShareUrlByID(fileID string) (*FileShare, error) {
	var resp RespInfo[FileShare]
	_, err := d.doupload(func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"task":    "22",
			"file_id": fileID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp.Info, nil
}

/*
通过分享链接获取数据
*/

// 判断类容
var isFileReg = regexp.MustCompile(`class="fileinfo"|id="file"|文件描述`)
var isFolderReg = regexp.MustCompile(`id="infos"`)

// 获取文件文件夹基础信息

// 获取文件名称
var nameFindReg = regexp.MustCompile(`<title>(.+?) - 蓝奏云</title>|id="filenajax">(.+?)</div>|var filename = '(.+?)';|<div style="font-size.+?>([^<>].+?)</div>|<div class="filethetext".+?>([^<>]+?)</div>`)

// 获取文件大小
var sizeFindReg = regexp.MustCompile(`(?i)大小\W*([0-9.]+\s*[bkm]+)`)

// 获取文件时间
var timeFindReg = regexp.MustCompile(`\d+\s*[秒天分小][钟时]?前|[昨前]天|\d{4}-\d{2}-\d{2}`)

// 查找分享文件夹子文件夹ID和名称
var findSubFolderReg = regexp.MustCompile(`(?i)(?:folderlink|mbxfolder).+href="/(.+?)"(?:.+filename")?>(.+?)<`)

// 获取下载页面链接
var findDownPageParamReg = regexp.MustCompile(`<iframe.*?src="(.+?)"`)

// 获取文件ID
var findFileIDReg = regexp.MustCompile(`'/ajaxm\.php\?file=(\d+)'`)

// 获取分享链接主界面
func (d *LanZou) getShareUrlHtml(shareID string) (string, error) {
	var vs string
	for i := 0; i < 3; i++ {
		firstPageData, err := d.get(fmt.Sprint(d.ShareUrl, "/", shareID),
			func(req *resty.Request) {
				if vs != "" {
					req.SetCookie(&http.Cookie{
						Name:  "acw_sc__v2",
						Value: vs,
					})
				}
			})
		if err != nil {
			return "", err
		}

		firstPageDataStr := RemoveNotes(string(firstPageData))
		if strings.Contains(firstPageDataStr, "取消分享") {
			return "", ErrFileShareCancel
		}
		if strings.Contains(firstPageDataStr, "文件不存在") {
			return "", ErrFileNotExist
		}

		// acw_sc__v2
		if strings.Contains(firstPageDataStr, "acw_sc__v2") {
			if vs, err = CalcAcwScV2(firstPageDataStr); err != nil {
				log.Errorf("lanzou: err => acw_sc__v2 validation error  ,data => %s\n", firstPageDataStr)
				return "", err
			}
			continue
		}
		return firstPageDataStr, nil
	}
	return "", errors.New("acw_sc__v2 validation error")
}

// 通过分享链接获取文件或文件夹
func (d *LanZou) GetFileOrFolderByShareUrl(shareID, pwd string) ([]model.Obj, error) {
	pageData, err := d.getShareUrlHtml(shareID)
	if err != nil {
		return nil, err
	}

	if !isFileReg.MatchString(pageData) {
		files, err := d.getFolderByShareUrl(pwd, pageData)
		if err != nil {
			return nil, err
		}
		return utils.MustSliceConvert(files, func(file FileOrFolderByShareUrl) model.Obj {
			return &file
		}), nil
	} else {
		file, err := d.getFilesByShareUrl(shareID, pwd, pageData)
		if err != nil {
			return nil, err
		}
		return []model.Obj{file}, nil
	}
}

// 通过分享链接获取文件(下载链接也使用此方法)
// FileOrFolderByShareUrl 包含 pwd 和 url 字段
// 参考 https://github.com/zaxtyson/LanZouCloud-API/blob/ab2e9ec715d1919bf432210fc16b91c6775fbb99/lanzou/api/core.py#L440
func (d *LanZou) GetFilesByShareUrl(shareID, pwd string) (file *FileOrFolderByShareUrl, err error) {
	pageData, err := d.getShareUrlHtml(shareID)
	if err != nil {
		return nil, err
	}
	return d.getFilesByShareUrl(shareID, pwd, pageData)
}

func (d *LanZou) getFilesByShareUrl(shareID, pwd string, sharePageData string) (*FileOrFolderByShareUrl, error) {
	var (
		param       map[string]string
		downloadUrl string
		baseUrl     string
		file        FileOrFolderByShareUrl
	)

	// 删除注释
	sharePageData = RemoveNotes(sharePageData)
	sharePageData = RemoveJSComment(sharePageData)

	// 需要密码
	if strings.Contains(sharePageData, "pwdload") || strings.Contains(sharePageData, "passwddiv") {
		sharePageData, err := getJSFunctionByName(sharePageData, "down_p")
		if err != nil {
			return nil, err
		}
		param, err := htmlJsonToMap(sharePageData)
		if err != nil {
			return nil, err
		}
		param["p"] = pwd

		fileIDs := findFileIDReg.FindStringSubmatch(sharePageData)
		var fileID string
		if len(fileIDs) > 1 {
			fileID = fileIDs[1]
		} else {
			return nil, fmt.Errorf("not find file id")
		}
		var resp FileShareInfoAndUrlResp[string]
		_, err = d.post(d.ShareUrl+"/ajaxm.php?file="+fileID, func(req *resty.Request) { req.SetFormData(param) }, &resp)
		if err != nil {
			return nil, err
		}
		file.NameAll = resp.Inf
		file.Pwd = pwd
		baseUrl = resp.GetBaseUrl()
		downloadUrl = resp.GetDownloadUrl()
	} else {
		urlpaths := findDownPageParamReg.FindStringSubmatch(sharePageData)
		if len(urlpaths) != 2 {
			log.Errorf("lanzou: err => not find file page param ,data => %s\n", sharePageData)
			return nil, fmt.Errorf("not find file page param")
		}
		data, err := d.get(fmt.Sprint(d.ShareUrl, urlpaths[1]), nil)
		if err != nil {
			return nil, err
		}
		nextPageData := RemoveNotes(string(data))
		param, err = htmlJsonToMap(nextPageData)
		if err != nil {
			return nil, err
		}

		fileIDs := findFileIDReg.FindStringSubmatch(nextPageData)
		var fileID string
		if len(fileIDs) > 1 {
			fileID = fileIDs[1]
		} else {
			return nil, fmt.Errorf("not find file id")
		}
		var resp FileShareInfoAndUrlResp[int]
		_, err = d.post(d.ShareUrl+"/ajaxm.php?file="+fileID, func(req *resty.Request) { req.SetFormData(param) }, &resp)
		if err != nil {
			return nil, err
		}
		baseUrl = resp.GetBaseUrl()
		downloadUrl = resp.GetDownloadUrl()

		names := nameFindReg.FindStringSubmatch(sharePageData)
		if len(names) > 1 {
			for _, name := range names[1:] {
				if name != "" {
					file.NameAll = name
					break
				}
			}
		}
	}

	sizes := sizeFindReg.FindStringSubmatch(sharePageData)
	if len(sizes) == 2 {
		file.Size = sizes[1]
	}
	file.ID = shareID
	file.Time = timeFindReg.FindString(sharePageData)

	// 重定向获取真实链接
	res, err := base.NoRedirectClient.R().SetHeaders(map[string]string{
		"accept-language": "zh-CN,zh;q=0.9,en;q=0.8,en-GB;q=0.7,en-US;q=0.6",
	}).Get(downloadUrl)
	if err != nil {
		return nil, err
	}

	file.Url = res.Header().Get("location")

	// 触发验证
	rPageData := res.String()
	if res.StatusCode() != 302 {
		param, err = htmlJsonToMap(rPageData)
		if err != nil {
			return nil, err
		}
		param["el"] = "2"
		time.Sleep(time.Second * 2)

		// 通过验证获取直连
		data, err := d.post(fmt.Sprint(baseUrl, "/ajax.php"), func(req *resty.Request) { req.SetFormData(param) }, nil)
		if err != nil {
			return nil, err
		}
		file.Url = utils.Json.Get(data, "url").ToString()
	}
	return &file, nil
}

// 通过分享链接获取文件夹
// 似乎子目录和文件不会加密
// 参考 https://github.com/zaxtyson/LanZouCloud-API/blob/ab2e9ec715d1919bf432210fc16b91c6775fbb99/lanzou/api/core.py#L1089
func (d *LanZou) GetFolderByShareUrl(shareID, pwd string) ([]FileOrFolderByShareUrl, error) {
	pageData, err := d.getShareUrlHtml(shareID)
	if err != nil {
		return nil, err
	}
	return d.getFolderByShareUrl(pwd, pageData)
}

func (d *LanZou) getFolderByShareUrl(pwd string, sharePageData string) ([]FileOrFolderByShareUrl, error) {
	from, err := htmlJsonToMap(sharePageData)
	if err != nil {
		return nil, err
	}

	files := make([]FileOrFolderByShareUrl, 0)
	// vip获取文件夹
	floders := findSubFolderReg.FindAllStringSubmatch(sharePageData, -1)
	for _, floder := range floders {
		if len(floder) == 3 {
			files = append(files, FileOrFolderByShareUrl{
				// Pwd: pwd, // 子文件夹不加密
				ID:       floder[1],
				NameAll:  floder[2],
				IsFloder: true,
			})
		}
	}

	// 获取文件
	from["pwd"] = pwd
	for page := 1; ; page++ {
		from["pg"] = strconv.Itoa(page)
		var resp FileOrFolderByShareUrlResp
		_, err := d.post(d.ShareUrl+"/filemoreajax.php", func(req *resty.Request) { req.SetFormData(from) }, &resp)
		if err != nil {
			return nil, err
		}
		// 文件夹中的文件加密
		for i := 0; i < len(resp.Text); i++ {
			resp.Text[i].Pwd = pwd
		}
		if len(resp.Text) == 0 {
			break
		}
		files = append(files, resp.Text...)
		time.Sleep(time.Second)
	}
	return files, nil
}

// 通过下载头获取真实文件信息
func (d *LanZou) getFileRealInfo(downURL string) (*int64, *time.Time) {
	res, _ := base.RestyClient.R().Head(downURL)
	if res == nil {
		return nil, nil
	}
	time, _ := http.ParseTime(res.Header().Get("Last-Modified"))
	size, _ := strconv.ParseInt(res.Header().Get("Content-Length"), 10, 64)
	return &size, &time
}

func (d *LanZou) getVeiAndUid() (vei string, uid string, err error) {
	var resp []byte
	resp, err = d.get("https://pc.woozooo.com/mydisk.php", func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"item":   "files",
			"action": "index",
		})
	})
	if err != nil {
		return
	}
	// uid
	uids := regexp.MustCompile(`uid=([^'"&;]+)`).FindStringSubmatch(string(resp))
	if len(uids) < 2 {
		err = fmt.Errorf("uid variable not find")
		return
	}
	uid = uids[1]

	// vei
	html := RemoveNotes(string(resp))
	data, err := htmlJsonToMap(html)
	if err != nil {
		return
	}
	vei = data["vei"]

	return
}
