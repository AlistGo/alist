package guangyapan

import "time"

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Sub          string `json:"sub"`
	Error        string `json:"error"`
	ErrorCode    int    `json:"error_code"`
	ErrorDesc    string `json:"error_description"`
}

type verificationResp struct {
	VerificationID string `json:"verification_id"`
	Error          string `json:"error"`
	ErrorCode      int    `json:"error_code"`
	ErrorDesc      string `json:"error_description"`
}

type captchaInitResp struct {
	CaptchaToken string `json:"captcha_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	ErrorCode    int    `json:"error_code"`
	ErrorDesc    string `json:"error_description"`
}

type verifyResp struct {
	VerificationToken string `json:"verification_token"`
	Error             string `json:"error"`
	ErrorCode         int    `json:"error_code"`
	ErrorDesc         string `json:"error_description"`
}

type userMeResp struct {
	Sub string `json:"sub"`
}

type listResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Total int        `json:"total"`
		List  []fileItem `json:"list"`
	} `json:"data"`
}

type fileItem struct {
	FileID   string `json:"fileId"`
	ParentID string `json:"parentId"`
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	ResType  int    `json:"resType"`
	CTime    int64  `json:"ctime"`
	UTime    int64  `json:"utime"`
}

type downloadResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		SignedURL   string `json:"signedURL"`
		DownloadURL string `json:"downloadUrl"`
	} `json:"data"`
}

type createDirResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
		ResType  int    `json:"resType"`
		CTime    int64  `json:"ctime"`
		UTime    int64  `json:"utime"`
	} `json:"data"`
}

type commonResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type deleteResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

type taskStatusResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Status int `json:"status"`
	} `json:"data"`
}

type uploadTokenResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data uploadTokenData `json:"data"`
}

type uploadTokenData struct {
	TaskID          string `json:"taskId"`
	ObjectPath      string `json:"objectPath"`
	Provider        any    `json:"provider"`
	Region          string `json:"region"`
	BucketName      string `json:"bucketName"`
	EndPoint        string `json:"endPoint"`
	FullEndPoint    string `json:"fullEndPoint"`
	CallbackVar     string `json:"callbackVar"`
	AccessKeyID     string `json:"accessKeyID"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	Creds           struct {
		AccessKeyID     string `json:"accessKeyID"`
		SecretAccessKey string `json:"secretAccessKey"`
		SessionToken    string `json:"sessionToken"`
	} `json:"creds"`
}

type taskInfoResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		FileID string `json:"fileId"`
	} `json:"data"`
}

type offlineResolveResp struct {
	Code int                `json:"code"`
	Msg  string             `json:"msg"`
	Data OfflineResolveData `json:"data"`
}

type OfflineResolveData struct {
	ResType   int               `json:"resType"`
	BTResInfo *OfflineBTResInfo `json:"btResInfo"`
	URL       string            `json:"url"`
}

type OfflineBTResInfo struct {
	InfoHash       string           `json:"infoHash"`
	FileName       string           `json:"fileName"`
	FileSize       int64            `json:"fileSize"`
	SubfilesNum    int              `json:"subfilesNum"`
	Subfiles       []OfflineSubfile `json:"subfiles"`
	CreateTime     int64            `json:"createTime"`
	ExcludeIndices []int            `json:"excludeIndices"`
}

type OfflineSubfile struct {
	FileName  string `json:"fileName"`
	FileIndex *int   `json:"fileIndex"`
	FileSize  int64  `json:"fileSize"`
}

type offlineCreateResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
		URL    string `json:"url"`
	} `json:"data"`
}

type offlineDeleteResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskIDs []string `json:"taskIds"`
	} `json:"data"`
}

type offlineListResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		StatusCounts []struct {
			Status int `json:"status"`
			Count  int `json:"count"`
		} `json:"statusCounts"`
		Cursor string        `json:"cursor"`
		List   []OfflineTask `json:"list"`
		Total  int           `json:"total"`
	} `json:"data"`
}

type OfflineTask struct {
	TaskID     string `json:"taskId"`
	FileName   string `json:"fileName"`
	TotalSize  int64  `json:"totalSize"`
	Status     int    `json:"status"`
	CreateTime int64  `json:"createTime"`
	Res        string `json:"res"`
	ResType    int    `json:"resType"`
	Progress   int    `json:"progress"`
	FileID     string `json:"fileId"`
	IsDir      bool   `json:"isDir"`
	Exist      bool   `json:"exist"`
}

func unixOrZero(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0)
}
