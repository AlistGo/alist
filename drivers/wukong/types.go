package wukong

type filterFileResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		FileList []wukongFile `json:"file_list"`
		HasMore  any          `json:"has_more"`
	} `json:"data"`
}

type wukongFile struct {
	FileID      int64  `json:"file_id"`
	FatherID    int64  `json:"father_id"`
	IsDirectory int    `json:"is_directory"`
	FileType    int    `json:"file_type"`
	Size        int64  `json:"size"`
	FileName    string `json:"file_name"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type rawResp struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

type uploadAuthTokenResp struct {
	Code            int    `json:"code"`
	Message         string `json:"message"`
	CurrentTime     int64  `json:"current_time"`
	ExpireTime      int64  `json:"expire_time"`
	SpaceName       string `json:"space_name"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
}

type vodResponseMetadata struct {
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

type vodDomain struct {
	Name    string `json:"Name"`
	Sign    string `json:"Sign"`
	StoreID string `json:"StoreID"`
}

type getUploadCandidatesResp struct {
	ResponseMetadata vodResponseMetadata `json:"ResponseMetadata"`
	Result           struct {
		Candidates []struct {
			Domains []vodDomain `json:"Domains"`
		} `json:"Candidates"`
		Domains []vodDomain `json:"Domains"`
	} `json:"Result"`
}

type applyUploadInnerResp struct {
	ResponseMetadata vodResponseMetadata `json:"ResponseMetadata"`
	Result           struct {
		InnerUploadAddress struct {
			UploadNodes []struct {
				StoreInfos []struct {
					StoreURI      string         `json:"StoreUri"`
					Auth          string         `json:"Auth"`
					UploadID      string         `json:"UploadID"`
					StorageHeader map[string]any `json:"StorageHeader"`
				} `json:"StoreInfos"`
				UploadHost string `json:"UploadHost"`
				SessionKey string `json:"SessionKey"`
			} `json:"UploadNodes"`
		} `json:"InnerUploadAddress"`
	} `json:"Result"`
}

type tosUploadResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Crc32      string `json:"crc32"`
		UploadID   string `json:"uploadid"`
		PartNumber string `json:"part_number"`
		Etag       string `json:"etag"`
	} `json:"data"`
}

type commitUploadInnerResp struct {
	ResponseMetadata vodResponseMetadata `json:"ResponseMetadata"`
	Result           struct {
		Results []struct {
			URI       string `json:"Uri"`
			URIStatus int    `json:"UriStatus"`
			Vid       string `json:"Vid"`
		} `json:"Results"`
		PluginResult any `json:"PluginResult"`
	} `json:"Result"`
}

type uploadSubmitResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
