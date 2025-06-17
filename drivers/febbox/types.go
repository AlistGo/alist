package febbox

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	hash_extend "github.com/alist-org/alist/v3/pkg/utils/hash"
	"strconv"
	"time"
)

type ErrResp struct {
	ErrorCode     int64   `json:"code"`
	ErrorMsg      string  `json:"msg"`
	ServerRunTime float64 `json:"server_runtime"`
	ServerName    string  `json:"server_name"`
}

func (e *ErrResp) IsError() bool {
	return e.ErrorCode != 0 || e.ErrorMsg != "" || e.ServerRunTime != 0 || e.ServerName != ""
}

func (e *ErrResp) Error() string {
	return fmt.Sprintf("ErrorCode: %d ,Error: %s ,ServerRunTime: %f ,ServerName: %s", e.ErrorCode, e.ErrorMsg, e.ServerRunTime, e.ServerName)
}

type FileListResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		FileList []File `json:"file_list"`
		ShowType string `json:"show_type"`
	} `json:"data"`
}

type Rules struct {
	AllowCopy     int64 `json:"allow_copy"`
	AllowDelete   int64 `json:"allow_delete"`
	AllowDownload int64 `json:"allow_download"`
	AllowComment  int64 `json:"allow_comment"`
	HideLocation  int64 `json:"hide_location"`
}

type File struct {
	Fid              int64  `json:"fid"`
	UID              int64  `json:"uid"`
	FileSize         int64  `json:"file_size"`
	Path             string `json:"path"`
	FileName         string `json:"file_name"`
	Ext              string `json:"ext"`
	AddTime          int64  `json:"add_time"`
	FileCreateTime   int64  `json:"file_create_time"`
	FileUpdateTime   int64  `json:"file_update_time"`
	ParentID         int64  `json:"parent_id"`
	UpdateTime       int64  `json:"update_time"`
	LastOpenTime     int64  `json:"last_open_time"`
	IsDir            int64  `json:"is_dir"`
	Epub             int64  `json:"epub"`
	IsMusicList      int64  `json:"is_music_list"`
	OssFid           int64  `json:"oss_fid"`
	Faststart        int64  `json:"faststart"`
	HasVideoQuality  int64  `json:"has_video_quality"`
	TotalDownload    int64  `json:"total_download"`
	Status           int64  `json:"status"`
	Remark           string `json:"remark"`
	OldHash          string `json:"old_hash"`
	Hash             string `json:"hash"`
	HashType         string `json:"hash_type"`
	FromUID          int64  `json:"from_uid"`
	FidOrg           int64  `json:"fid_org"`
	ShareID          int64  `json:"share_id"`
	InvitePermission int64  `json:"invite_permission"`
	ThumbSmall       string `json:"thumb_small"`
	ThumbSmallWidth  int64  `json:"thumb_small_width"`
	ThumbSmallHeight int64  `json:"thumb_small_height"`
	Thumb            string `json:"thumb"`
	ThumbWidth       int64  `json:"thumb_width"`
	ThumbHeight      int64  `json:"thumb_height"`
	ThumbBig         string `json:"thumb_big"`
	ThumbBigWidth    int64  `json:"thumb_big_width"`
	ThumbBigHeight   int64  `json:"thumb_big_height"`
	IsCustomThumb    int64  `json:"is_custom_thumb"`
	Photos           int64  `json:"photos"`
	IsAlbum          int64  `json:"is_album"`
	ReadOnly         int64  `json:"read_only"`
	Rules            Rules  `json:"rules"`
	IsShared         int64  `json:"is_shared"`
}

func fileToObj(f File) *model.ObjThumb {
	return &model.ObjThumb{
		Object: model.Object{
			ID:       strconv.FormatInt(f.Fid, 10),
			Name:     f.FileName,
			Size:     f.FileSize,
			Ctime:    time.Unix(f.FileCreateTime, 0),
			Modified: time.Unix(f.FileUpdateTime, 0),
			IsFolder: f.IsDir == 1,
			HashInfo: utils.NewHashInfo(hash_extend.GCID, f.Hash),
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: f.Thumb,
		},
	}
}

type FileDownloadResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Error       int    `json:"error"`
		DownloadURL string `json:"download_url"`
		Hash        string `json:"hash"`
		HashType    string `json:"hash_type"`
		Fid         int    `json:"fid"`
		FileName    string `json:"file_name"`
		ParentID    int    `json:"parent_id"`
		FileSize    int    `json:"file_size"`
		Ext         string `json:"ext"`
		Thumb       string `json:"thumb"`
		VipLink     int    `json:"vip_link"`
	} `json:"data"`
}
