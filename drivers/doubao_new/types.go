package doubao_new

import "github.com/alist-org/alist/v3/internal/model"

type BaseResp struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg,omitempty"`
	Message string `json:"message,omitempty"`
}

type ListResp struct {
	BaseResp
	Data ListData `json:"data"`
}

type ListData struct {
	HasMore   bool     `json:"has_more"`
	LastLabel string   `json:"last_label"`
	NodeList  []string `json:"node_list"`
	Entities  struct {
		Nodes map[string]Node `json:"nodes"`
		Users map[string]User `json:"users"`
	} `json:"entities"`
}

type Node struct {
	Token      string `json:"token"`
	NodeToken  string `json:"node_token"`
	ObjToken   string `json:"obj_token"`
	Name       string `json:"name"`
	Type       int    `json:"type"`
	NodeType   int    `json:"node_type"`
	OwnerID    string `json:"owner_id"`
	EditUID    string `json:"edit_uid"`
	CreateTime int64  `json:"create_time"`
	EditTime   int64  `json:"edit_time"`
	URL        string `json:"url"`
	Extra      struct {
		Size string `json:"size"`
	} `json:"extra"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Object struct {
	model.Object
	ObjToken string
	NodeType int
	ObjType  int
	URL      string
}

type CreateFolderResp struct {
	BaseResp
	Data struct {
		Entities struct {
			Nodes map[string]Node `json:"nodes"`
		} `json:"entities"`
		NodeList []string `json:"node_list"`
	} `json:"data"`
}

type FileInfoResp struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    FileInfo `json:"data"`
}

type FileInfo struct {
	Name        string      `json:"name"`
	NumBlocks   int         `json:"num_blocks"`
	Version     string      `json:"version"`
	MimeType    string      `json:"mime_type"`
	MountPoint  string      `json:"mount_point"`
	PreviewMeta PreviewMeta `json:"preview_meta"`
}

type PreviewMeta struct {
	Data map[string]PreviewMetaEntry `json:"data"`
}

type PreviewMetaEntry struct {
	Status          int    `json:"status"`
	Extra           string `json:"extra"`
	PreviewFileSize int64  `json:"preview_file_size"`
}

type PreviewImageExtra struct {
	ImgExt   string `json:"img_ext"`
	PageNums int    `json:"page_nums"`
}

type UserStorageResp struct {
	BaseResp
	Data UserStorageData `json:"data"`
}

type UserStorageData struct {
	ShowSizeLimit       bool  `json:"show_size_limit"`
	TotalSizeLimitBytes int64 `json:"total_size_limit_bytes"`
	UsedSizeBytes       int64 `json:"used_size_bytes"`
}

type UploadPrepareResp struct {
	BaseResp
	Data UploadPrepareData `json:"data"`
}

type UploadPrepareData struct {
	BlockSize       int64  `json:"block_size"`
	NumBlocks       int    `json:"num_blocks"`
	OptionBlockSize int64  `json:"option_block_size"`
	DedupeSupport   bool   `json:"dedupe_support"`
	UploadID        string `json:"upload_id"`
}

type UploadBlock struct {
	Hash       string `json:"hash"`
	Seq        int    `json:"seq"`
	Size       int64  `json:"size"`
	Checksum   string `json:"checksum"`
	IsUploaded bool   `json:"isUploaded"`
}

type UploadBlocksResp struct {
	BaseResp
	Data UploadBlocksData `json:"data"`
}

type UploadBlocksData struct {
	NeededUploadBlocks []UploadBlockNeed `json:"needed_upload_blocks"`
}

type UploadBlockNeed struct {
	Seq      int    `json:"seq"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
	Hash     string `json:"hash"`
}

type UploadMergeResp struct {
	BaseResp
	Data UploadMergeData `json:"data"`
}

type UploadMergeData struct {
	SuccessSeqList []int `json:"success_seq_list"`
}

type UploadFinishResp struct {
	BaseResp
	Data UploadFinishData `json:"data"`
}

type UploadFinishData struct {
	Version     string `json:"version"`
	DataVersion string `json:"data_version"`
	Extra       struct {
		NodeToken string `json:"node_token"`
	} `json:"extra"`
	FileToken string `json:"file_token"`
}

type RemoveResp struct {
	BaseResp
	Data struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type TaskStatusResp struct {
	BaseResp
	Data TaskStatusData `json:"data"`
}

type TaskStatusData struct {
	IsFinish bool `json:"is_finish"`
	IsFail   bool `json:"is_fail"`
}
