package doubao

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

type BaseResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type NodeInfoResp struct {
	BaseResp
	Data struct {
		NodeInfo   File   `json:"node_info"`
		Children   []File `json:"children"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
	} `json:"data"`
}

type File struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Key                 string `json:"key"`
	NodeType            int    `json:"node_type"` // 0: 文件, 1: 文件夹
	Size                int64  `json:"size"`
	Source              int    `json:"source"`
	NameReviewStatus    int    `json:"name_review_status"`
	ContentReviewStatus int    `json:"content_review_status"`
	RiskReviewStatus    int    `json:"risk_review_status"`
	ConversationID      string `json:"conversation_id"`
	ParentID            string `json:"parent_id"`
	CreateTime          int64  `json:"create_time"`
	UpdateTime          int64  `json:"update_time"`
}

type GetDownloadInfoResp struct {
	BaseResp
	Data struct {
		DownloadInfos []struct {
			NodeID    string `json:"node_id"`
			MainURL   string `json:"main_url"`
			BackupURL string `json:"backup_url"`
		} `json:"download_infos"`
	} `json:"data"`
}

type GetFileUrlResp struct {
	BaseResp
	Data struct {
		FileUrls []struct {
			URI     string `json:"uri"`
			MainURL string `json:"main_url"`
			BackURL string `json:"back_url"`
		} `json:"file_urls"`
	} `json:"data"`
}

type GetVideoFileUrlResp struct {
	BaseResp
	Data struct {
		MediaType string `json:"media_type"`
		MediaInfo []struct {
			Meta struct {
				Height     string  `json:"height"`
				Width      string  `json:"width"`
				Format     string  `json:"format"`
				Duration   float64 `json:"duration"`
				CodecType  string  `json:"codec_type"`
				Definition string  `json:"definition"`
			} `json:"meta"`
			MainURL   string `json:"main_url"`
			BackupURL string `json:"backup_url"`
		} `json:"media_info"`
		OriginalMediaInfo struct {
			Meta struct {
				Height     string  `json:"height"`
				Width      string  `json:"width"`
				Format     string  `json:"format"`
				Duration   float64 `json:"duration"`
				CodecType  string  `json:"codec_type"`
				Definition string  `json:"definition"`
			} `json:"meta"`
			MainURL   string `json:"main_url"`
			BackupURL string `json:"backup_url"`
		} `json:"original_media_info"`
		PosterURL      string `json:"poster_url"`
		PlayableStatus int    `json:"playable_status"`
	} `json:"data"`
}

type UploadNodeResp struct {
	BaseResp
	Data struct {
		NodeList []struct {
			LocalID  string `json:"local_id"`
			ID       string `json:"id"`
			ParentID string `json:"parent_id"`
			Name     string `json:"name"`
			Key      string `json:"key"`
			NodeType int    `json:"node_type"` // 0: 文件, 1: 文件夹
		} `json:"node_list"`
	} `json:"data"`
}

type Object struct {
	model.Object
	Key      string
	NodeType int
}

type UserInfoResp struct {
	Data    UserInfo `json:"data"`
	Message string   `json:"message"`
}
type AppUserInfo struct {
	BuiAuditInfo string `json:"bui_audit_info"`
}
type AuditInfo struct {
}
type Details struct {
}
type BuiAuditInfo struct {
	AuditInfo      AuditInfo `json:"audit_info"`
	IsAuditing     bool      `json:"is_auditing"`
	AuditStatus    int       `json:"audit_status"`
	LastUpdateTime int       `json:"last_update_time"`
	UnpassReason   string    `json:"unpass_reason"`
	Details        Details   `json:"details"`
}
type Connects struct {
	Platform           string `json:"platform"`
	ProfileImageURL    string `json:"profile_image_url"`
	ExpiredTime        int    `json:"expired_time"`
	ExpiresIn          int    `json:"expires_in"`
	PlatformScreenName string `json:"platform_screen_name"`
	UserID             int64  `json:"user_id"`
	PlatformUID        string `json:"platform_uid"`
	SecPlatformUID     string `json:"sec_platform_uid"`
	PlatformAppID      int    `json:"platform_app_id"`
	ModifyTime         int    `json:"modify_time"`
	AccessToken        string `json:"access_token"`
	OpenID             string `json:"open_id"`
}
type OperStaffRelationInfo struct {
	HasPassword               int    `json:"has_password"`
	Mobile                    string `json:"mobile"`
	SecOperStaffUserID        string `json:"sec_oper_staff_user_id"`
	RelationMobileCountryCode int    `json:"relation_mobile_country_code"`
}
type UserInfo struct {
	AppID                 int                   `json:"app_id"`
	AppUserInfo           AppUserInfo           `json:"app_user_info"`
	AvatarURL             string                `json:"avatar_url"`
	BgImgURL              string                `json:"bg_img_url"`
	BuiAuditInfo          BuiAuditInfo          `json:"bui_audit_info"`
	CanBeFoundByPhone     int                   `json:"can_be_found_by_phone"`
	Connects              []Connects            `json:"connects"`
	CountryCode           int                   `json:"country_code"`
	Description           string                `json:"description"`
	DeviceID              int                   `json:"device_id"`
	Email                 string                `json:"email"`
	EmailCollected        bool                  `json:"email_collected"`
	Gender                int                   `json:"gender"`
	HasPassword           int                   `json:"has_password"`
	HmRegion              int                   `json:"hm_region"`
	IsBlocked             int                   `json:"is_blocked"`
	IsBlocking            int                   `json:"is_blocking"`
	IsRecommendAllowed    int                   `json:"is_recommend_allowed"`
	IsVisitorAccount      bool                  `json:"is_visitor_account"`
	Mobile                string                `json:"mobile"`
	Name                  string                `json:"name"`
	NeedCheckBindStatus   bool                  `json:"need_check_bind_status"`
	OdinUserType          int                   `json:"odin_user_type"`
	OperStaffRelationInfo OperStaffRelationInfo `json:"oper_staff_relation_info"`
	PhoneCollected        bool                  `json:"phone_collected"`
	RecommendHintMessage  string                `json:"recommend_hint_message"`
	ScreenName            string                `json:"screen_name"`
	SecUserID             string                `json:"sec_user_id"`
	SessionKey            string                `json:"session_key"`
	UseHmRegion           bool                  `json:"use_hm_region"`
	UserCreateTime        int                   `json:"user_create_time"`
	UserID                int64                 `json:"user_id"`
	UserIDStr             string                `json:"user_id_str"`
	UserVerified          bool                  `json:"user_verified"`
	VerifiedContent       string                `json:"verified_content"`
}

// UploadToken 上传令牌配置
type UploadToken struct {
	Alice    map[string]UploadAuthToken
	Samantha MediaUploadAuthToken
}

// UploadAuthToken 多种类型的上传配置：图片/文件
type UploadAuthToken struct {
	ServiceID        string `json:"service_id"`
	UploadPathPrefix string `json:"upload_path_prefix"`
	Auth             struct {
		AccessKeyID     string    `json:"access_key_id"`
		SecretAccessKey string    `json:"secret_access_key"`
		SessionToken    string    `json:"session_token"`
		ExpiredTime     time.Time `json:"expired_time"`
		CurrentTime     time.Time `json:"current_time"`
	} `json:"auth"`
	UploadHost string `json:"upload_host"`
}

// MediaUploadAuthToken 媒体上传配置
type MediaUploadAuthToken struct {
	StsToken struct {
		AccessKeyID     string    `json:"access_key_id"`
		SecretAccessKey string    `json:"secret_access_key"`
		SessionToken    string    `json:"session_token"`
		ExpiredTime     time.Time `json:"expired_time"`
		CurrentTime     time.Time `json:"current_time"`
	} `json:"sts_token"`
	UploadInfo struct {
		VideoHost string `json:"video_host"`
		SpaceName string `json:"space_name"`
	} `json:"upload_info"`
}

type UploadAuthTokenResp struct {
	BaseResp
	Data UploadAuthToken `json:"data"`
}

type MediaUploadAuthTokenResp struct {
	BaseResp
	Data MediaUploadAuthToken `json:"data"`
}

type ResponseMetadata struct {
	RequestID string `json:"RequestId"`
	Action    string `json:"Action"`
	Version   string `json:"Version"`
	Service   string `json:"Service"`
	Region    string `json:"Region"`
	Error     struct {
		CodeN   int    `json:"CodeN,omitempty"`
		Code    string `json:"Code,omitempty"`
		Message string `json:"Message,omitempty"`
	} `json:"Error,omitempty"`
}

type UploadConfig struct {
	UploadAddress         UploadAddress         `json:"UploadAddress"`
	FallbackUploadAddress FallbackUploadAddress `json:"FallbackUploadAddress"`
	InnerUploadAddress    InnerUploadAddress    `json:"InnerUploadAddress"`
	RequestID             string                `json:"RequestId"`
	SDKParam              interface{}           `json:"SDKParam"`
}

type UploadConfigResp struct {
	ResponseMetadata `json:"ResponseMetadata"`
	Result           UploadConfig `json:"Result"`
}

// StoreInfo 存储信息
type StoreInfo struct {
	StoreURI      string                 `json:"StoreUri"`
	Auth          string                 `json:"Auth"`
	UploadID      string                 `json:"UploadID"`
	UploadHeader  map[string]interface{} `json:"UploadHeader,omitempty"`
	StorageHeader map[string]interface{} `json:"StorageHeader,omitempty"`
}

// UploadAddress 上传地址信息
type UploadAddress struct {
	StoreInfos   []StoreInfo            `json:"StoreInfos"`
	UploadHosts  []string               `json:"UploadHosts"`
	UploadHeader map[string]interface{} `json:"UploadHeader"`
	SessionKey   string                 `json:"SessionKey"`
	Cloud        string                 `json:"Cloud"`
}

// FallbackUploadAddress 备用上传地址
type FallbackUploadAddress struct {
	StoreInfos   []StoreInfo            `json:"StoreInfos"`
	UploadHosts  []string               `json:"UploadHosts"`
	UploadHeader map[string]interface{} `json:"UploadHeader"`
	SessionKey   string                 `json:"SessionKey"`
	Cloud        string                 `json:"Cloud"`
}

// UploadNode 上传节点信息
type UploadNode struct {
	Vid          string                 `json:"Vid"`
	Vids         []string               `json:"Vids"`
	StoreInfos   []StoreInfo            `json:"StoreInfos"`
	UploadHost   string                 `json:"UploadHost"`
	UploadHeader map[string]interface{} `json:"UploadHeader"`
	Type         string                 `json:"Type"`
	Protocol     string                 `json:"Protocol"`
	SessionKey   string                 `json:"SessionKey"`
	NodeConfig   struct {
		UploadMode string `json:"UploadMode"`
	} `json:"NodeConfig"`
	Cluster string `json:"Cluster"`
}

// AdvanceOption 高级选项
type AdvanceOption struct {
	Parallel      int    `json:"Parallel"`
	Stream        int    `json:"Stream"`
	SliceSize     int    `json:"SliceSize"`
	EncryptionKey string `json:"EncryptionKey"`
}

// InnerUploadAddress 内部上传地址
type InnerUploadAddress struct {
	UploadNodes   []UploadNode  `json:"UploadNodes"`
	AdvanceOption AdvanceOption `json:"AdvanceOption"`
}

// UploadPart 上传分片信息
type UploadPart struct {
	UploadId   string `json:"uploadid,omitempty"`
	PartNumber string `json:"part_number,omitempty"`
	Crc32      string `json:"crc32,omitempty"`
	Etag       string `json:"etag,omitempty"`
	Mode       string `json:"mode,omitempty"`
}

// UploadResp 上传响应体
type UploadResp struct {
	Code       int        `json:"code"`
	ApiVersion string     `json:"apiversion"`
	Message    string     `json:"message"`
	Data       UploadPart `json:"data"`
}

type VideoCommitUpload struct {
	Vid       string `json:"Vid"`
	VideoMeta struct {
		URI          string  `json:"Uri"`
		Height       int     `json:"Height"`
		Width        int     `json:"Width"`
		OriginHeight int     `json:"OriginHeight"`
		OriginWidth  int     `json:"OriginWidth"`
		Duration     float64 `json:"Duration"`
		Bitrate      int     `json:"Bitrate"`
		Md5          string  `json:"Md5"`
		Format       string  `json:"Format"`
		Size         int     `json:"Size"`
		FileType     string  `json:"FileType"`
		Codec        string  `json:"Codec"`
	} `json:"VideoMeta"`
	WorkflowInput struct {
		TemplateID string `json:"TemplateId"`
	} `json:"WorkflowInput"`
	GetPosterMode string `json:"GetPosterMode"`
}

type VideoCommitUploadResp struct {
	ResponseMetadata ResponseMetadata `json:"ResponseMetadata"`
	Result           struct {
		RequestID string              `json:"RequestId"`
		Results   []VideoCommitUpload `json:"Results"`
	} `json:"Result"`
}

type CommonResp struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg,omitempty"`
	Message string          `json:"message,omitempty"` // 错误情况下的消息
	Data    json.RawMessage `json:"data,omitempty"`    // 原始数据,稍后解析
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Locale  string `json:"locale"`
	} `json:"error,omitempty"`
}

// IsSuccess 判断响应是否成功
func (r *CommonResp) IsSuccess() bool {
	return r.Code == 0
}

// GetError 获取错误信息
func (r *CommonResp) GetError() error {
	if r.IsSuccess() {
		return nil
	}
	// 优先使用message字段
	errMsg := r.Message
	if errMsg == "" {
		errMsg = r.Msg
	}
	// 如果error对象存在且有详细消息,则使用error中的信息
	if r.Error != nil && r.Error.Message != "" {
		errMsg = r.Error.Message
	}

	return fmt.Errorf("[doubao] API error (code: %d): %s", r.Code, errMsg)
}

// UnmarshalData 将data字段解析为指定类型
func (r *CommonResp) UnmarshalData(v interface{}) error {
	if !r.IsSuccess() {
		return r.GetError()
	}

	if len(r.Data) == 0 {
		return nil
	}

	return json.Unmarshal(r.Data, v)
}
