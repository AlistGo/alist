package yunpan360

import (
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

const (
	authTypeCookie     = "cookie"
	authTypeAPIKey     = "api_key"
	openEnvProd        = "prod"
	defaultSubChannel  = "open"
	openSignSecret     = "e7b24b112a44fdd9ee93bdf998c6ca0e"
	openClientID       = "e4757e933b6486c08ed206ecb6d5d9e684fcb4e2"
	openClientSecret   = "885fd3231f1c1e37c9f462261a09b8c38cde0c2b"
	openClientSecretQA = "b11b8fff1c75a5d227c8cc93aaeb0bb70c8eee47"
)

type BaseResp struct {
	Errno  int    `json:"errno"`
	Errmsg string `json:"errmsg"`
}

type CookieDownloadSession struct {
	OwnerQID string
	Token    string
}

type ListResp interface {
	Objects(parentPath string) []model.Obj
	GetHasNextPage() bool
}

type CookieListResp struct {
	BaseResp
	Token       string     `json:"token"`
	OwnerQid    string     `json:"owner_qid"`
	Qid         string     `json:"qid"`
	Data        []ListItem `json:"data"`
	HasNextPage bool       `json:"has_next_page"`
}

func (r *CookieListResp) Objects(parentPath string) []model.Obj {
	ownerQID := r.GetOwnerQID()
	return utils.MustSliceConvert(r.Data, func(src ListItem) model.Obj {
		return src.toObj(parentPath, ownerQID, r.Token)
	})
}

func (r *CookieListResp) GetHasNextPage() bool {
	return r.HasNextPage
}

func (r *CookieListResp) GetOwnerQID() string {
	return firstNonEmpty(r.OwnerQid, r.Qid)
}

type ListItem struct {
	NID        string `json:"nid"`
	FileName   string `json:"file_name"`
	FilePath   string `json:"file_path"`
	FileSize   string `json:"file_size"`
	IsDir      bool   `json:"is_dir"`
	Fhash      string `json:"fhash"`
	CreateTime string `json:"create_time"`
	ModifyTime string `json:"modify_time"`
	Mtime      string `json:"mtime"`
	ServerTime string `json:"server_time"`
	Preview    string `json:"preview"`
	Thumb      string `json:"thumb"`
	SrcPic     string `json:"srcpic"`
	OwnerQid   string `json:"owner_qid"`
	Qid        string `json:"qid"`
	Token      string `json:"token"`
}

func (i ListItem) toObj(parentPath, ownerQID, token string) model.Obj {
	objPath := normalizeRemotePath(i.FilePath)
	if objPath == "" || !pathLooksLikeObject(objPath, i.FileName) {
		objPath = joinRemotePath(parentPath, i.FileName)
	}
	thumb := ""
	if !i.IsDir {
		thumb = absoluteURL(firstNonEmpty(i.Thumb, i.SrcPic, i.Preview))
	}

	return &YunpanObject{
		Object: model.Object{
			ID:       i.NID,
			Path:     objPath,
			Name:     i.FileName,
			Size:     parseSize(i.FileSize),
			Modified: parseYunpanTime(i.ModifyTime, i.Mtime),
			Ctime:    parseYunpanTime(i.CreateTime, i.ServerTime),
			IsFolder: i.IsDir,
			HashInfo: parseHash(i.Fhash),
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: thumb,
		},
		OwnerQID:      firstNonEmpty(i.OwnerQid, i.Qid, ownerQID),
		DownloadToken: firstNonEmpty(i.Token, token),
	}
}

func parseSize(raw string) int64 {
	size, _ := strconv.ParseInt(raw, 10, 64)
	return size
}

func parseHash(raw string) utils.HashInfo {
	if len(raw) == 40 {
		return utils.NewHashInfo(utils.SHA1, raw)
	}
	return utils.HashInfo{}
}

func parseYunpanTime(unixStr, text string) time.Time {
	if t := parseUnixTime(unixStr); !t.IsZero() {
		return t
	}
	return parseTextTime(text)
}

func parseUnixTime(raw string) time.Time {
	if raw != "" {
		sec, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && sec > 0 {
			return time.Unix(sec, 0)
		}
	}
	return time.Time{}
}

func parseTextTime(text string) time.Time {
	if text == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", text, utils.CNLoc)
	if err == nil {
		return t
	}
	return time.Time{}
}

func normalizeRemotePath(p string) string {
	if p == "" {
		return ""
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return utils.FixAndCleanPath(p)
}

type OpenAuthResp struct {
	BaseResp
	Data struct {
		Token             string `json:"token"`
		AccessToken       string `json:"access_token"`
		AccessTokenExpire int64  `json:"access_token_expire"`
		Qid               string `json:"qid"`
	} `json:"data"`
}

type OpenAuthInfo struct {
	AccessToken string
	Qid         string
	Token       string
	SubChannel  string
}

type OpenListResp struct {
	BaseResp
	Data struct {
		NodeList   []OpenNode `json:"node_list"`
		List       []OpenNode `json:"list"`
		Data       []OpenNode `json:"data"`
		TotalCount int        `json:"total_count"`
		Total      int        `json:"total"`
		PageNum    int        `json:"page_num"`
	} `json:"data"`
}

func (r *OpenListResp) Objects(parentPath string) []model.Obj {
	nodes := r.Data.NodeList
	if len(nodes) == 0 {
		nodes = r.Data.List
	}
	if len(nodes) == 0 {
		nodes = r.Data.Data
	}
	return utils.MustSliceConvert(nodes, func(src OpenNode) model.Obj {
		return src.toObj(parentPath)
	})
}

func (r *OpenListResp) GetHasNextPage() bool {
	total := r.Data.TotalCount
	if total <= 0 {
		total = r.Data.Total
	}
	if total <= 0 {
		return false
	}
	loaded := len(r.Data.NodeList)
	if loaded == 0 {
		loaded = len(r.Data.List)
	}
	if loaded == 0 {
		loaded = len(r.Data.Data)
	}
	return loaded > 0 && loaded < total
}

type OpenNode struct {
	NID        string      `json:"nid"`
	Name       string      `json:"name"`
	FName      string      `json:"fname"`
	Path       string      `json:"path"`
	FPath      string      `json:"fpath"`
	Type       interface{} `json:"type"`
	IsDir      interface{} `json:"is_dir"`
	CountSize  interface{} `json:"count_size"`
	Size       interface{} `json:"size"`
	CreateTime interface{} `json:"create_time"`
	ModifyTime interface{} `json:"modify_time"`
	MTime      interface{} `json:"mtime"`
	FileHash   string      `json:"file_hash"`
	Fhash      string      `json:"fhash"`
	Thumb      string      `json:"thumb"`
	Preview    string      `json:"preview"`
	SrcPic     string      `json:"srcpic"`
}

func (n OpenNode) toObj(parentPath string) model.Obj {
	name := firstNonEmpty(strings.TrimSpace(n.Name), strings.TrimSpace(n.FName))
	objPath := normalizeRemotePath(firstNonEmpty(n.FPath, n.Path))
	if objPath == "" || !pathLooksLikeObject(objPath, name) {
		objPath = joinRemotePath(parentPath, name)
	}
	isDir := parseOpenDir(n.IsDir, n.Type)
	thumb := ""
	if !isDir {
		thumb = absoluteURL(firstNonEmpty(n.Thumb, n.SrcPic, n.Preview))
	}

	return &YunpanObject{
		Object: model.Object{
			ID:       n.NID,
			Path:     objPath,
			Name:     name,
			Size:     parseAnySize(n.CountSize, n.Size),
			Modified: parseAnyTime(n.ModifyTime, n.MTime),
			Ctime:    parseAnyTime(n.CreateTime),
			IsFolder: isDir,
			HashInfo: parseHash(firstNonEmpty(n.FileHash, n.Fhash)),
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: thumb,
		},
	}
}

type OpenUserInfoResp struct {
	BaseResp
	Data map[string]interface{} `json:"data"`
}

type OpenMkdirResp struct {
	BaseResp
	Data struct {
		NID string `json:"nid"`
	} `json:"data"`
}

type CookieMkdirResp struct {
	BaseResp
	Data struct {
		NID string `json:"nid"`
	} `json:"data"`
}

type CookieMoveResp struct {
	BaseResp
	Data struct {
		TaskID  string `json:"task_id"`
		IsAsync bool   `json:"is_async"`
	} `json:"data"`
}

type CookieRecycleResp struct {
	BaseResp
	Data struct {
		TaskID  string `json:"task_id"`
		IsAsync bool   `json:"is_async"`
	} `json:"data"`
}

type CookieAsyncQueryResp struct {
	BaseResp
	Data map[string]CookieAsyncTask `json:"data"`
}

type CookieAsyncTask struct {
	MessageID string `json:"message_id"`
	SendTime  string `json:"send_time"`
	Status    int    `json:"status"`
	Action    string `json:"action"`
	Errno     int    `json:"errno"`
	Errstr    string `json:"errstr"`
	Result    string `json:"result"`
}

type CookieDownloadResp struct {
	BaseResp
	Data struct {
		DownloadURL string `json:"download_url"`
		Store       string `json:"store"`
		Host        string `json:"host"`
	} `json:"data"`
}

func (r *CookieDownloadResp) GetURL() string {
	return r.Data.DownloadURL
}

type OpenDownloadResp struct {
	BaseResp
	Data struct {
		DownloadURL string `json:"downloadUrl"`
	} `json:"data"`
	DownloadURL string `json:"downloadUrl"`
}

func (r *OpenDownloadResp) GetURL() string {
	return firstNonEmpty(r.Data.DownloadURL, r.DownloadURL)
}

type YunpanObject struct {
	model.Object
	model.Thumbnail
	OwnerQID      string
	DownloadToken string
}

func parseAnySize(values ...interface{}) int64 {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if v == "" {
				continue
			}
			size, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return size
			}
		case float64:
			return int64(v)
		case int64:
			return v
		case int:
			return int64(v)
		}
	}
	return 0
}

func parseAnyTime(values ...interface{}) time.Time {
	for _, value := range values {
		switch v := value.(type) {
		case string:
			if t := parseUnixTime(v); !t.IsZero() {
				return t
			}
			if t := parseTextTime(v); !t.IsZero() {
				return t
			}
		case float64:
			if v > 0 {
				return time.Unix(int64(v), 0)
			}
		case int64:
			if v > 0 {
				return time.Unix(v, 0)
			}
		case int:
			if v > 0 {
				return time.Unix(int64(v), 0)
			}
		}
	}
	return time.Time{}
}

func parseOpenDir(values ...interface{}) bool {
	for _, value := range values {
		switch v := value.(type) {
		case bool:
			if v {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "true", "dir", "folder":
				return true
			}
		case float64:
			if int64(v) == 1 {
				return true
			}
		case int64:
			if v == 1 {
				return true
			}
		case int:
			if v == 1 {
				return true
			}
		}
	}
	return false
}

func pathLooksLikeObject(objPath, name string) bool {
	if objPath == "" || name == "" {
		return false
	}
	return strings.TrimSuffix(stdPathBase(objPath), "/") == strings.TrimSuffix(name, "/")
}

func stdPathBase(p string) string {
	if p == "/" {
		return "/"
	}
	idx := strings.LastIndex(strings.TrimSuffix(p, "/"), "/")
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}

func joinRemotePath(parentPath, name string) string {
	parentPath = normalizeRemotePath(parentPath)
	if parentPath == "" {
		parentPath = "/"
	}
	return normalizeRemotePath(strings.TrimSuffix(parentPath, "/") + "/" + strings.TrimPrefix(name, "/"))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
