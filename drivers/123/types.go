package _123

import (
	"github.com/alist-org/alist/v3/pkg/utils"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

type File struct {
	FileName    string    `json:"FileName"`
	Size        int64     `json:"Size"`
	UpdateAt    time.Time `json:"UpdateAt"`
	FileId      int64     `json:"FileId"`
	Type        int       `json:"Type"`
	Etag        string    `json:"Etag"`
	S3KeyFlag   string    `json:"S3KeyFlag"`
	DownloadUrl string    `json:"DownloadUrl"`
	IsLock      bool      `json:"IsLock"`
}

func (f File) CreateTime() time.Time {
	return f.UpdateAt
}

func (f File) GetHash() utils.HashInfo {
	return utils.HashInfo{}
}

func (f File) GetPath() string {
	return ""
}

func (f File) GetSize() int64 {
	return f.Size
}

func (f File) GetName() string {
	return f.FileName
}

func (f File) ModTime() time.Time {
	return f.UpdateAt
}

func (f File) IsDir() bool {
	return f.Type == 1
}

func (f File) GetID() string {
	return strconv.FormatInt(f.FileId, 10)
}

func (f File) Thumb() string {
	if f.DownloadUrl == "" {
		return ""
	}
	du, err := url.Parse(f.DownloadUrl)
	if err != nil {
		return ""
	}
	du.Path = strings.TrimSuffix(du.Path, "_24_24") + "_70_70"
	query := du.Query()
	query.Set("w", "70")
	query.Set("h", "70")
	if !query.Has("type") {
		query.Set("type", strings.TrimPrefix(path.Base(f.FileName), "."))
	}
	if !query.Has("trade_key") {
		query.Set("trade_key", "123pan-thumbnail")
	}
	du.RawQuery = query.Encode()
	return du.String()
}

var _ model.Obj = (*File)(nil)
var _ model.Thumb = (*File)(nil)

//func (f File) Thumb() string {
//
//}
//var _ model.Thumb = (*File)(nil)

type Files struct {
	//BaseResp
	Data struct {
		Next     string `json:"Next"`
		Total    int    `json:"Total"`
		InfoList []File `json:"InfoList"`
	} `json:"data"`
}

//type DownResp struct {
//	//BaseResp
//	Data struct {
//		DownloadUrl string `json:"DownloadUrl"`
//	} `json:"data"`
//}

type UploadResp struct {
	//BaseResp
	Data struct {
		AccessKeyId     string `json:"AccessKeyId"`
		Bucket          string `json:"Bucket"`
		Key             string `json:"Key"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
		FileId          int64  `json:"FileId"`
		Reuse           bool   `json:"Reuse"`
		EndPoint        string `json:"EndPoint"`
		StorageNode     string `json:"StorageNode"`
		UploadId        string `json:"UploadId"`
	} `json:"data"`
}

type S3PreSignedURLs struct {
	Data struct {
		PreSignedUrls map[string]string `json:"presignedUrls"`
	} `json:"data"`
}
