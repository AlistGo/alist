package _123_open

import (
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type ApiInfo struct {
	url   string
	qps   int
	token chan struct{}
}

func (a *ApiInfo) Require() {
	if a.qps > 0 {
		a.token <- struct{}{}
	}
}
func (a *ApiInfo) Release() {
	if a.qps > 0 {
		time.AfterFunc(time.Second, func() {
			<-a.token
		})
	}
}
func (a *ApiInfo) SetQPS(qps int) {
	a.qps = qps
	a.token = make(chan struct{}, qps)
}
func (a *ApiInfo) NowLen() int {
	return len(a.token)
}
func InitApiInfo(url string, qps int) *ApiInfo {
	return &ApiInfo{
		url:   url,
		qps:   qps,
		token: make(chan struct{}, qps),
	}
}

type File struct {
	FileName     string `json:"filename"`
	Size         int64  `json:"size"`
	CreateAt     string `json:"createAt"`
	UpdateAt     string `json:"updateAt"`
	FileId       int64  `json:"fileId"`
	Type         int    `json:"type"`
	Etag         string `json:"etag"`
	S3KeyFlag    string `json:"s3KeyFlag"`
	ParentFileId int    `json:"parentFileId"`
	Category     int    `json:"category"`
	Status       int    `json:"status"`
	Trashed      int    `json:"trashed"`
}

func (f File) GetHash() utils.HashInfo {
	return utils.NewHashInfo(utils.MD5, f.Etag)
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

func (f File) CreateTime() time.Time {
	parsedTime, err := time.Parse("2006-01-02 15:04:05", f.CreateAt)
	if err != nil {
		return time.Now()
	}
	return parsedTime
}

func (f File) ModTime() time.Time {
	parsedTime, err := time.Parse("2006-01-02 15:04:05", f.UpdateAt)
	if err != nil {
		return time.Now()
	}
	return parsedTime
}

func (f File) IsDir() bool {
	return f.Type == 1
}

func (f File) GetID() string {
	return strconv.FormatInt(f.FileId, 10)
}

var _ model.Obj = (*File)(nil)

type BaseResp struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	XTraceID string `json:"x-traceID"`
}

type AccessTokenResp struct {
	BaseResp
	Data struct {
		AccessToken string `json:"accessToken"`
		ExpiredAt   string `json:"expiredAt"`
	} `json:"data"`
}

type RefreshTokenResp struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type UserInfoResp struct {
	BaseResp
	Data struct {
		UID            int64  `json:"uid"`
		Username       string `json:"username"`
		DisplayName    string `json:"displayName"`
		HeadImage      string `json:"headImage"`
		Passport       string `json:"passport"`
		Mail           string `json:"mail"`
		SpaceUsed      int64  `json:"spaceUsed"`
		SpacePermanent int64  `json:"spacePermanent"`
		SpaceTemp      int64  `json:"spaceTemp"`
		SpaceTempExpr  string `json:"spaceTempExpr"`
		Vip            bool   `json:"vip"`
		DirectTraffic  int64  `json:"directTraffic"`
		IsHideUID      bool   `json:"isHideUID"`
	} `json:"data"`
}

type FileListResp struct {
	BaseResp
	Data struct {
		LastFileId int64  `json:"lastFileId"`
		FileList   []File `json:"fileList"`
	} `json:"data"`
}

type DownloadInfoResp struct {
	BaseResp
	Data struct {
		DownloadUrl string `json:"downloadUrl"`
	} `json:"data"`
}

type UploadCreateResp struct {
	BaseResp
	Data struct {
		FileID      int64  `json:"fileID"`
		PreuploadID string `json:"preuploadID"`
		Reuse       bool   `json:"reuse"`
		SliceSize   int64  `json:"sliceSize"`
	} `json:"data"`
}

type UploadUrlResp struct {
	BaseResp
	Data struct {
		PresignedURL string `json:"presignedURL"`
	}
}

type UploadCompleteResp struct {
	BaseResp
	Data struct {
		Async     bool  `json:"async"`
		Completed bool  `json:"completed"`
		FileID    int64 `json:"fileID"`
	} `json:"data"`
}

type UploadAsyncResp struct {
	BaseResp
	Data struct {
		Completed bool  `json:"completed"`
		FileID    int64 `json:"fileID"`
	} `json:"data"`
}

type UploadResp struct {
	BaseResp
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
