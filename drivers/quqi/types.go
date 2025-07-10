package quqi

type BaseReqQuery struct {
	ID string `json:"quqiid"`
}

type BaseReq struct {
	GroupID string `json:"quqi_id"`
}

type BaseRes struct {
	//Data    interface{} `json:"data"`
	Code    int    `json:"err"`
	Message string `json:"msg"`
}

type GroupRes struct {
	BaseRes
	Data []*Group `json:"data"`
}

type ListRes struct {
	BaseRes
	Data *List `json:"data"`
}

type GetDocRes struct {
	BaseRes
	Data struct {
		OriginPath string `json:"origin_path"`
	} `json:"data"`
}

type GetDownloadResp struct {
	BaseRes
	Data struct {
		Url string `json:"url"`
	} `json:"data"`
}

type MakeDirRes struct {
	BaseRes
	Data struct {
		IsRoot   bool  `json:"is_root"`
		NodeID   int64 `json:"node_id"`
		ParentID int64 `json:"parent_id"`
	} `json:"data"`
}

type MoveRes struct {
	BaseRes
	Data struct {
		NodeChildNum int64  `json:"node_child_num"`
		NodeID       int64  `json:"node_id"`
		NodeName     string `json:"node_name"`
		ParentID     int64  `json:"parent_id"`
		GroupID      int64  `json:"quqi_id"`
		TreeID       int64  `json:"tree_id"`
	} `json:"data"`
}

type RenameRes struct {
	BaseRes
	Data struct {
		NodeID     int64  `json:"node_id"`
		GroupID    int64  `json:"quqi_id"`
		Rename     string `json:"rename"`
		TreeID     int64  `json:"tree_id"`
		UpdateTime int64  `json:"updatetime"`
	} `json:"data"`
}

type CopyRes struct {
	BaseRes
}

type RemoveRes struct {
	BaseRes
}

type Group struct {
	ID              int    `json:"quqi_id"`
	Type            int    `json:"type"`
	Name            string `json:"name"`
	IsAdministrator int    `json:"is_administrator"`
	Role            []int  `json:"role"`
	Avatar          string `json:"avatar_url"`
	IsStick         int    `json:"is_stick"`
	Nickname        string `json:"nickname"`
	Status          int    `json:"status"`
}

type List struct {
	ListDir
	Dir  []*ListDir  `json:"dir"`
	File []*ListFile `json:"file"`
}

type ListItem struct {
	AddTime        int64  `json:"add_time"`
	IsDir          int    `json:"is_dir"`
	IsExpand       int    `json:"is_expand"`
	IsFinalize     int    `json:"is_finalize"`
	LastEditorName string `json:"last_editor_name"`
	Name           string `json:"name"`
	NodeID         int64  `json:"nid"`
	ParentID       int64  `json:"parent_id"`
	Permission     int    `json:"permission"`
	TreeID         int64  `json:"tid"`
	UpdateCNT      int64  `json:"update_cnt"`
	UpdateTime     int64  `json:"update_time"`
}

type ListDir struct {
	ListItem
	ChildDocNum int64  `json:"child_doc_num"`
	DirDetail   string `json:"dir_detail"`
	DirType     int    `json:"dir_type"`
}

type ListFile struct {
	ListItem
	BroadDocType       string `json:"broad_doc_type"`
	CanDisplay         bool   `json:"can_display"`
	Detail             string `json:"detail"`
	EXT                string `json:"ext"`
	Filetype           string `json:"filetype"`
	HasMobileThumbnail bool   `json:"has_mobile_thumbnail"`
	HasThumbnail       bool   `json:"has_thumbnail"`
	Size               int64  `json:"size"`
	Version            int    `json:"version"`
}

type UploadInitResp struct {
	Data struct {
		Bucket   string `json:"bucket"`
		Exist    bool   `json:"exist"`
		Key      string `json:"key"`
		TaskID   string `json:"task_id"`
		Token    string `json:"token"`
		UploadID string `json:"upload_id"`
		URL      string `json:"url"`
		NodeID   int64  `json:"node_id"`
		NodeName string `json:"node_name"`
		ParentID int64  `json:"parent_id"`
	} `json:"data"`
	Err int    `json:"err"`
	Msg string `json:"msg"`
}

type TempKeyResp struct {
	Err  int    `json:"err"`
	Msg  string `json:"msg"`
	Data struct {
		ExpiredTime int    `json:"expiredTime"`
		Expiration  string `json:"expiration"`
		Credentials struct {
			SessionToken string `json:"sessionToken"`
			TmpSecretID  string `json:"tmpSecretId"`
			TmpSecretKey string `json:"tmpSecretKey"`
		} `json:"credentials"`
		RequestID string `json:"requestId"`
		StartTime int    `json:"startTime"`
	} `json:"data"`
}

type UploadFinishResp struct {
	Data struct {
		NodeID   int64  `json:"node_id"`
		NodeName string `json:"node_name"`
		ParentID int64  `json:"parent_id"`
		QuqiID   int64  `json:"quqi_id"`
		TreeID   int64  `json:"tree_id"`
	} `json:"data"`
	Err int    `json:"err"`
	Msg string `json:"msg"`
}

type UrlExchangeResp struct {
	BaseRes
	Data struct {
		Name               string `json:"name"`
		Mime               string `json:"mime"`
		Size               int64  `json:"size"`
		DownloadType       int    `json:"download_type"`
		ChannelType        int    `json:"channel_type"`
		ChannelID          int    `json:"channel_id"`
		Url                string `json:"url"`
		ExpiredTime        int64  `json:"expired_time"`
		IsEncrypted        bool   `json:"is_encrypted"`
		EncryptedSize      int64  `json:"encrypted_size"`
		EncryptedAlg       string `json:"encrypted_alg"`
		EncryptedKey       string `json:"encrypted_key"`
		PassportID         int64  `json:"passport_id"`
		RequestExpiredTime int64  `json:"request_expired_time"`
	} `json:"data"`
}
