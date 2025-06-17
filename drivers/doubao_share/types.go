package doubao_share

import (
	"encoding/json"
	"fmt"
	"github.com/alist-org/alist/v3/internal/model"
)

type BaseResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type NodeInfoData struct {
	Share      ShareInfo   `json:"share,omitempty"`
	Creator    CreatorInfo `json:"creator,omitempty"`
	NodeList   []File      `json:"node_list,omitempty"`
	NodeInfo   File        `json:"node_info,omitempty"`
	Children   []File      `json:"children,omitempty"`
	Path       FilePath    `json:"path,omitempty"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more,omitempty"`
}

type NodeInfoResp struct {
	BaseResp
	NodeInfoData `json:"data"`
}

type RootFileList struct {
	ShareID     string
	VirtualPath string
	NodeInfo    NodeInfoData
	Child       *[]RootFileList
}

type File struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Key                 string `json:"key"`
	NodeType            int    `json:"node_type"`
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

type FileObject struct {
	model.Object
	ShareID  string
	Key      string
	NodeID   string
	NodeType int
}

type ShareInfo struct {
	ShareID   string `json:"share_id"`
	FirstNode struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Key      string `json:"key"`
		NodeType int    `json:"node_type"`
		Size     int    `json:"size"`
		Source   int    `json:"source"`
		Content  struct {
			LinkFileType  string `json:"link_file_type"`
			ImageWidth    int    `json:"image_width"`
			ImageHeight   int    `json:"image_height"`
			AiSkillStatus int    `json:"ai_skill_status"`
		} `json:"content"`
		NameReviewStatus    int    `json:"name_review_status"`
		ContentReviewStatus int    `json:"content_review_status"`
		RiskReviewStatus    int    `json:"risk_review_status"`
		ConversationID      string `json:"conversation_id"`
		ParentID            string `json:"parent_id"`
		CreateTime          int    `json:"create_time"`
		UpdateTime          int    `json:"update_time"`
	} `json:"first_node"`
	NodeCount      int    `json:"node_count"`
	CreateTime     int    `json:"create_time"`
	Channel        string `json:"channel"`
	InfluencerType int    `json:"influencer_type"`
}

type CreatorInfo struct {
	EntityID string `json:"entity_id"`
	UserName string `json:"user_name"`
	NickName string `json:"nick_name"`
	Avatar   struct {
		OriginURL string `json:"origin_url"`
		TinyURL   string `json:"tiny_url"`
		URI       string `json:"uri"`
	} `json:"avatar"`
}

type FilePath []struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Key                 string `json:"key"`
	NodeType            int    `json:"node_type"`
	Size                int    `json:"size"`
	Source              int    `json:"source"`
	NameReviewStatus    int    `json:"name_review_status"`
	ContentReviewStatus int    `json:"content_review_status"`
	RiskReviewStatus    int    `json:"risk_review_status"`
	ConversationID      string `json:"conversation_id"`
	ParentID            string `json:"parent_id"`
	CreateTime          int    `json:"create_time"`
	UpdateTime          int    `json:"update_time"`
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
