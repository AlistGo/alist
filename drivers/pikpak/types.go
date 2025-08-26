package pikpak

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	hash_extend "github.com/alist-org/alist/v3/pkg/utils/hash"
)

type Files struct {
	Files         []File `json:"files"`
	NextPageToken string `json:"next_page_token"`
}

type File struct {
	Id             string    `json:"id"`
	Kind           string    `json:"kind"`
	Name           string    `json:"name"`
	CreatedTime    time.Time `json:"created_time"`
	ModifiedTime   time.Time `json:"modified_time"`
	Hash           string    `json:"hash"`
	Size           string    `json:"size"`
	ThumbnailLink  string    `json:"thumbnail_link"`
	WebContentLink string    `json:"web_content_link"`
	Medias         []Media   `json:"medias"`
}

func fileToObj(f File) *model.ObjThumb {
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	return &model.ObjThumb{
		Object: model.Object{
			ID:       f.Id,
			Name:     f.Name,
			Size:     size,
			Ctime:    f.CreatedTime,
			Modified: f.ModifiedTime,
			IsFolder: f.Kind == "drive#folder",
			HashInfo: utils.NewHashInfo(hash_extend.GCID, f.Hash),
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: f.ThumbnailLink,
		},
	}
}

type Media struct {
	MediaId   string `json:"media_id"`
	MediaName string `json:"media_name"`
	Video     struct {
		Height     int    `json:"height"`
		Width      int    `json:"width"`
		Duration   int    `json:"duration"`
		BitRate    int    `json:"bit_rate"`
		FrameRate  int    `json:"frame_rate"`
		VideoCodec string `json:"video_codec"`
		AudioCodec string `json:"audio_codec"`
		VideoType  string `json:"video_type"`
	} `json:"video"`
	Link struct {
		Url    string    `json:"url"`
		Token  string    `json:"token"`
		Expire time.Time `json:"expire"`
	} `json:"link"`
	NeedMoreQuota  bool          `json:"need_more_quota"`
	VipTypes       []interface{} `json:"vip_types"`
	RedirectLink   string        `json:"redirect_link"`
	IconLink       string        `json:"icon_link"`
	IsDefault      bool          `json:"is_default"`
	Priority       int           `json:"priority"`
	IsOrigin       bool          `json:"is_origin"`
	ResolutionName string        `json:"resolution_name"`
	IsVisible      bool          `json:"is_visible"`
	Category       string        `json:"category"`
}

type UploadTaskData struct {
	UploadType string `json:"upload_type"`
	//UPLOAD_TYPE_RESUMABLE
	Resumable *struct {
		Kind     string   `json:"kind"`
		Params   S3Params `json:"params"`
		Provider string   `json:"provider"`
	} `json:"resumable"`

	File File `json:"file"`
}

type S3Params struct {
	AccessKeyID     string    `json:"access_key_id"`
	AccessKeySecret string    `json:"access_key_secret"`
	Bucket          string    `json:"bucket"`
	Endpoint        string    `json:"endpoint"`
	Expiration      time.Time `json:"expiration"`
	Key             string    `json:"key"`
	SecurityToken   string    `json:"security_token"`
}

// 添加离线下载响应
type OfflineDownloadResp struct {
	File       *string     `json:"file"`
	Task       OfflineTask `json:"task"`
	UploadType string      `json:"upload_type"`
	URL        struct {
		Kind string `json:"kind"`
	} `json:"url"`
}

// 离线下载列表
type OfflineListResp struct {
	ExpiresIn     int64         `json:"expires_in"`
	NextPageToken string        `json:"next_page_token"`
	Tasks         []OfflineTask `json:"tasks"`
}

// offlineTask
type OfflineTask struct {
	Callback          string            `json:"callback"`
	CreatedTime       string            `json:"created_time"`
	FileID            string            `json:"file_id"`
	FileName          string            `json:"file_name"`
	FileSize          string            `json:"file_size"`
	IconLink          string            `json:"icon_link"`
	ID                string            `json:"id"`
	Kind              string            `json:"kind"`
	Message           string            `json:"message"`
	Name              string            `json:"name"`
	Params            Params            `json:"params"`
	Phase             string            `json:"phase"` // PHASE_TYPE_RUNNING, PHASE_TYPE_ERROR, PHASE_TYPE_COMPLETE, PHASE_TYPE_PENDING
	Progress          int64             `json:"progress"`
	ReferenceResource ReferenceResource `json:"reference_resource"`
	Space             string            `json:"space"`
	StatusSize        int64             `json:"status_size"`
	Statuses          []string          `json:"statuses"`
	ThirdTaskID       string            `json:"third_task_id"`
	Type              string            `json:"type"`
	UpdatedTime       string            `json:"updated_time"`
	UserID            string            `json:"user_id"`
}

type Params struct {
	Age         string  `json:"age"`
	MIMEType    *string `json:"mime_type,omitempty"`
	PredictType string  `json:"predict_type"`
	URL         string  `json:"url"`
}

type ReferenceResource struct {
	Type          string                 `json:"@type"`
	Audit         interface{}            `json:"audit"`
	Hash          string                 `json:"hash"`
	IconLink      string                 `json:"icon_link"`
	ID            string                 `json:"id"`
	Kind          string                 `json:"kind"`
	Medias        []Media                `json:"medias"`
	MIMEType      string                 `json:"mime_type"`
	Name          string                 `json:"name"`
	Params        map[string]interface{} `json:"params"`
	ParentID      string                 `json:"parent_id"`
	Phase         string                 `json:"phase"`
	Size          string                 `json:"size"`
	Space         string                 `json:"space"`
	Starred       bool                   `json:"starred"`
	Tags          []string               `json:"tags"`
	ThumbnailLink string                 `json:"thumbnail_link"`
}

type ErrResp struct {
	ErrorCode        int64  `json:"error_code"`
	ErrorMsg         string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (e *ErrResp) IsError() bool {
	return e.ErrorCode != 0 || e.ErrorMsg != "" || e.ErrorDescription != ""
}

func (e *ErrResp) Error() string {
	return fmt.Sprintf("ErrorCode: %d ,Error: %s ,ErrorDescription: %s ", e.ErrorCode, e.ErrorMsg, e.ErrorDescription)
}

type CaptchaTokenRequest struct {
	Action       string            `json:"action"`
	CaptchaToken string            `json:"captcha_token"`
	ClientID     string            `json:"client_id"`
	DeviceID     string            `json:"device_id"`
	Meta         map[string]string `json:"meta"`
	RedirectUri  string            `json:"redirect_uri"`
}

type CaptchaTokenResponse struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Url          string `json:"url"`
}

type CaptchaApiResponse struct {
	Token string `json:"token"`
	Code  int64  `json:"code"`
	Url   string `json:"url_received"`
}
