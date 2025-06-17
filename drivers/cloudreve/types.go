package cloudreve

import (
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

type Resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type Policy struct {
	Id       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	MaxSize  int      `json:"max_size"`
	FileType []string `json:"file_type"`
}

type UploadInfo struct {
	SessionID   string   `json:"sessionID"`
	ChunkSize   int      `json:"chunkSize"`
	Expires     int      `json:"expires"`
	UploadURLs  []string `json:"uploadURLs"`
	Credential  string   `json:"credential,omitempty"`  // local
	CompleteURL string   `json:"completeURL,omitempty"` // s3
}

type DirectoryResp struct {
	Parent  string   `json:"parent"`
	Objects []Object `json:"objects"`
	Policy  Policy   `json:"policy"`
}

type Object struct {
	Id            string    `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Pic           string    `json:"pic"`
	Size          int       `json:"size"`
	Type          string    `json:"type"`
	Date          time.Time `json:"date"`
	CreateDate    time.Time `json:"create_date"`
	SourceEnabled bool      `json:"source_enabled"`
}

type DirectoryProp struct {
	Size int `json:"size"`
}

func objectToObj(f Object, t model.Thumbnail) *model.ObjThumb {
	return &model.ObjThumb{
		Object: model.Object{
			ID:       f.Id,
			Name:     f.Name,
			Size:     int64(f.Size),
			Modified: f.Date,
			IsFolder: f.Type == "dir",
		},
		Thumbnail: t,
	}
}

type Config struct {
	LoginCaptcha bool   `json:"loginCaptcha"`
	CaptchaType  string `json:"captcha_type"`
}
