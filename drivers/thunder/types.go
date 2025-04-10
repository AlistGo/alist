package thunder

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	hash_extend "github.com/alist-org/alist/v3/pkg/utils/hash"
)

type ErrResp struct {
	ErrorCode        int64  `json:"error_code"`
	ErrorMsg         string `json:"error"`
	ErrorDescription string `json:"error_description"`
	//	ErrorDetails   interface{} `json:"error_details"`
}

func (e *ErrResp) IsError() bool {
	if e.ErrorMsg == "success" {
		return false
	}

	return e.ErrorCode != 0 || e.ErrorMsg != "" || e.ErrorDescription != ""
}

func (e *ErrResp) Error() string {
	return fmt.Sprintf("ErrorCode: %d ,Error: %s ,ErrorDescription: %s ", e.ErrorCode, e.ErrorMsg, e.ErrorDescription)
}

/*
* 验证码Token
**/
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

/*
* 登录
**/
type TokenResp struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`

	Sub    string `json:"sub"`
	UserID string `json:"user_id"`
}

func (t *TokenResp) Token() string {
	return fmt.Sprint(t.TokenType, " ", t.AccessToken)
}

type SignInRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`

	Provider    string `json:"provider"`
	SigninToken string `json:"signin_token"`
}

type CoreLoginRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	SequenceNo      string `json:"sequenceNo"`
	PlatformVersion string `json:"platformVersion"`
	IsCompressed    string `json:"isCompressed"`
	Appid           string `json:"appid"`
	ClientVersion   string `json:"clientVersion"`
	PeerID          string `json:"peerID"`
	AppName         string `json:"appName"`
	SdkVersion      string `json:"sdkVersion"`
	Devicesign      string `json:"devicesign"`
	NetWorkType     string `json:"netWorkType"`
	ProviderName    string `json:"providerName"`
	DeviceModel     string `json:"deviceModel"`
	DeviceName      string `json:"deviceName"`
	OSVersion       string `json:"OSVersion"`
	Creditkey       string `json:"creditkey"`
	Hl              string `json:"hl"`
	UserName        string `json:"userName"`
	PassWord        string `json:"passWord"`
	VerifyKey       string `json:"verifyKey"`
	VerifyCode      string `json:"verifyCode"`
	IsMd5Pwd        string `json:"isMd5Pwd"`
}

type CoreLoginResp struct {
	Account   string `json:"account"`
	Creditkey string `json:"creditkey"`
	/*	Error              string `json:"error"`
		ErrorCode          string `json:"errorCode"`
		ErrorDescription   string `json:"error_description"`*/
	ExpiresIn          int    `json:"expires_in"`
	IsCompressed       string `json:"isCompressed"`
	IsSetPassWord      string `json:"isSetPassWord"`
	KeepAliveMinPeriod string `json:"keepAliveMinPeriod"`
	KeepAlivePeriod    string `json:"keepAlivePeriod"`
	LoginKey           string `json:"loginKey"`
	NickName           string `json:"nickName"`
	PlatformVersion    string `json:"platformVersion"`
	ProtocolVersion    string `json:"protocolVersion"`
	SecureKey          string `json:"secureKey"`
	SequenceNo         string `json:"sequenceNo"`
	SessionID          string `json:"sessionID"`
	Timestamp          string `json:"timestamp"`
	UserID             string `json:"userID"`
	UserName           string `json:"userName"`
	UserNewNo          string `json:"userNewNo"`
	Version            string `json:"version"`
	/*	VipList []struct {
		ExpireDate string `json:"expireDate"`
		IsAutoDeduct string `json:"isAutoDeduct"`
		IsVip string `json:"isVip"`
		IsYear string `json:"isYear"`
		PayID string `json:"payId"`
		PayName string `json:"payName"`
		Register string `json:"register"`
		Vasid string `json:"vasid"`
		VasType string `json:"vasType"`
		VipDayGrow string `json:"vipDayGrow"`
		VipGrow string `json:"vipGrow"`
		VipLevel string `json:"vipLevel"`
		Icon struct {
			General string `json:"general"`
			Small string `json:"small"`
		} `json:"icon"`
	} `json:"vipList"`*/
}

/*
* 文件
**/
type FileList struct {
	Kind            string  `json:"kind"`
	NextPageToken   string  `json:"next_page_token"`
	Files           []Files `json:"files"`
	Version         string  `json:"version"`
	VersionOutdated bool    `json:"version_outdated"`
}

type Link struct {
	URL    string    `json:"url"`
	Token  string    `json:"token"`
	Expire time.Time `json:"expire"`
	Type   string    `json:"type"`
}

var _ model.Obj = (*Files)(nil)

type Files struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
	//UserID         string    `json:"user_id"`
	Size string `json:"size"`
	//Revision       string    `json:"revision"`
	//FileExtension  string    `json:"file_extension"`
	//MimeType       string    `json:"mime_type"`
	//Starred        bool      `json:"starred"`
	WebContentLink string    `json:"web_content_link"`
	CreatedTime    time.Time `json:"created_time"`
	ModifiedTime   time.Time `json:"modified_time"`
	IconLink       string    `json:"icon_link"`
	ThumbnailLink  string    `json:"thumbnail_link"`
	// Md5Checksum    string    `json:"md5_checksum"`
	Hash string `json:"hash"`
	// Links map[string]Link `json:"links"`
	// Phase string          `json:"phase"`
	// Audit struct {
	// 	Status  string `json:"status"`
	// 	Message string `json:"message"`
	// 	Title   string `json:"title"`
	// } `json:"audit"`
	Medias []struct {
		//Category       string `json:"category"`
		//IconLink       string `json:"icon_link"`
		//IsDefault      bool   `json:"is_default"`
		//IsOrigin       bool   `json:"is_origin"`
		//IsVisible      bool   `json:"is_visible"`
		Link Link `json:"link"`
		//MediaID        string `json:"media_id"`
		//MediaName      string `json:"media_name"`
		//NeedMoreQuota  bool   `json:"need_more_quota"`
		//Priority       int    `json:"priority"`
		//RedirectLink   string `json:"redirect_link"`
		//ResolutionName string `json:"resolution_name"`
		// Video          struct {
		// 	AudioCodec string `json:"audio_codec"`
		// 	BitRate    int    `json:"bit_rate"`
		// 	Duration   int    `json:"duration"`
		// 	FrameRate  int    `json:"frame_rate"`
		// 	Height     int    `json:"height"`
		// 	VideoCodec string `json:"video_codec"`
		// 	VideoType  string `json:"video_type"`
		// 	Width      int    `json:"width"`
		// } `json:"video"`
		// VipTypes []string `json:"vip_types"`
	} `json:"medias"`
	Trashed     bool   `json:"trashed"`
	DeleteTime  string `json:"delete_time"`
	OriginalURL string `json:"original_url"`
	//Params            struct{} `json:"params"`
	//OriginalFileIndex int    `json:"original_file_index"`
	//Space             string `json:"space"`
	//Apps              []interface{} `json:"apps"`
	//Writable   bool   `json:"writable"`
	//FolderType string `json:"folder_type"`
	//Collection interface{} `json:"collection"`
}

func (c *Files) GetHash() utils.HashInfo {
	return utils.NewHashInfo(hash_extend.GCID, c.Hash)
}

func (c *Files) GetSize() int64        { size, _ := strconv.ParseInt(c.Size, 10, 64); return size }
func (c *Files) GetName() string       { return c.Name }
func (c *Files) CreateTime() time.Time { return c.CreatedTime }
func (c *Files) ModTime() time.Time    { return c.ModifiedTime }
func (c *Files) IsDir() bool           { return c.Kind == FOLDER }
func (c *Files) GetID() string         { return c.ID }
func (c *Files) GetPath() string       { return "" }
func (c *Files) Thumb() string         { return c.ThumbnailLink }

/*
* 上传
**/
type UploadTaskResponse struct {
	UploadType string `json:"upload_type"`

	/*//UPLOAD_TYPE_FORM
	Form struct {
		//Headers struct{} `json:"headers"`
		Kind       string `json:"kind"`
		Method     string `json:"method"`
		MultiParts struct {
			OSSAccessKeyID string `json:"OSSAccessKeyId"`
			Signature      string `json:"Signature"`
			Callback       string `json:"callback"`
			Key            string `json:"key"`
			Policy         string `json:"policy"`
			XUserData      string `json:"x:user_data"`
		} `json:"multi_parts"`
		URL string `json:"url"`
	} `json:"form"`*/

	//UPLOAD_TYPE_RESUMABLE
	Resumable struct {
		Kind   string `json:"kind"`
		Params struct {
			AccessKeyID     string    `json:"access_key_id"`
			AccessKeySecret string    `json:"access_key_secret"`
			Bucket          string    `json:"bucket"`
			Endpoint        string    `json:"endpoint"`
			Expiration      time.Time `json:"expiration"`
			Key             string    `json:"key"`
			SecurityToken   string    `json:"security_token"`
		} `json:"params"`
		Provider string `json:"provider"`
	} `json:"resumable"`

	File Files `json:"file"`
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
	Callback    string   `json:"callback"`
	CreatedTime string   `json:"created_time"`
	FileID      string   `json:"file_id"`
	FileName    string   `json:"file_name"`
	FileSize    string   `json:"file_size"`
	IconLink    string   `json:"icon_link"`
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Message     string   `json:"message"`
	Name        string   `json:"name"`
	Params      Params   `json:"params"`
	Phase       string   `json:"phase"` // PHASE_TYPE_RUNNING, PHASE_TYPE_ERROR, PHASE_TYPE_COMPLETE, PHASE_TYPE_PENDING
	Progress    int64    `json:"progress"`
	Space       string   `json:"space"`
	StatusSize  int64    `json:"status_size"`
	Statuses    []string `json:"statuses"`
	ThirdTaskID string   `json:"third_task_id"`
	Type        string   `json:"type"`
	UpdatedTime string   `json:"updated_time"`
	UserID      string   `json:"user_id"`
}

type Params struct {
	FolderType   string `json:"folder_type"`
	PredictSpeed string `json:"predict_speed"`
	PredictType  string `json:"predict_type"`
}

// LoginReviewResp 登录验证响应
type LoginReviewResp struct {
	Creditkey        string `json:"creditkey"`
	Error            string `json:"error"`
	ErrorCode        string `json:"errorCode"`
	ErrorDesc        string `json:"errorDesc"`
	ErrorDescURL     string `json:"errorDescUrl"`
	ErrorIsRetry     int    `json:"errorIsRetry"`
	ErrorDescription string `json:"error_description"`
	IsCompressed     string `json:"isCompressed"`
	PlatformVersion  string `json:"platformVersion"`
	ProtocolVersion  string `json:"protocolVersion"`
	Reviewurl        string `json:"reviewurl"`
	SequenceNo       string `json:"sequenceNo"`
	UserID           string `json:"userID"`
	VerifyType       string `json:"verifyType"`
}

// ReviewData 验证数据
type ReviewData struct {
	Creditkey  string `json:"creditkey"`
	Reviewurl  string `json:"reviewurl"`
	Deviceid   string `json:"deviceid"`
	Devicesign string `json:"devicesign"`
}
