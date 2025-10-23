package bitqiu

import "encoding/json"

type Response[T any] struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type LoginData struct {
	UserID int64 `json:"userId"`
}

type ResourcePage struct {
	CurrentPage    int        `json:"currentPage"`
	PageSize       int        `json:"pageSize"`
	TotalCount     int        `json:"totalCount"`
	TotalPageCount int        `json:"totalPageCount"`
	Data           []Resource `json:"data"`
	HasNext        bool       `json:"hasNext"`
}

type Resource struct {
	ResourceID   string       `json:"resourceId"`
	ResourceUID  string       `json:"resourceUid"`
	ResourceType int          `json:"resourceType"`
	ParentID     string       `json:"parentId"`
	Name         string       `json:"name"`
	ExtName      string       `json:"extName"`
	Size         *json.Number `json:"size"`
	CreateTime   *string      `json:"createTime"`
	UpdateTime   *string      `json:"updateTime"`
	FileMD5      string       `json:"fileMd5"`
}

type DownloadData struct {
	URL  string `json:"url"`
	MD5  string `json:"md5"`
	Size int64  `json:"size"`
}

type UserInfoData struct {
	RootDirID string `json:"rootDirId"`
}

type CreateDirData struct {
	DirID    string `json:"dirId"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}

type AsyncManagerData struct {
	WaitTasks    []AsyncTask `json:"waitTaskList"`
	RunningTasks []AsyncTask `json:"runningTaskList"`
	SuccessTasks []AsyncTask `json:"successTaskList"`
	FailTasks    []AsyncTask `json:"failTaskList"`
	TaskList     []AsyncTask `json:"taskList"`
}

type AsyncTask struct {
	TaskID      string         `json:"taskId"`
	Status      int            `json:"status"`
	ErrorMsg    string         `json:"errorMsg"`
	Message     string         `json:"message"`
	Result      *AsyncTaskInfo `json:"result"`
	TargetName  string         `json:"targetName"`
	TargetDirID string         `json:"parentId"`
}

type AsyncTaskInfo struct {
	Resource Resource `json:"resource"`
	DirID    string   `json:"dirId"`
	FileID   string   `json:"fileId"`
	Name     string   `json:"name"`
	ParentID string   `json:"parentId"`
}

func (t AsyncTask) ErrorMessage() string {
	if t.ErrorMsg != "" {
		return t.ErrorMsg
	}
	if t.Message != "" {
		return t.Message
	}
	return "unknown error"
}

type UploadInitData struct {
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Token        string `json:"token"`
	FileUID      string `json:"fileUid"`
	FileSID      string `json:"fileSid"`
	ParentID     string `json:"parentId"`
	UserID       int64  `json:"userId"`
	SerialNumber string `json:"serialNumber"`
	UploadURL    string `json:"uploadUrl"`
	AppID        string `json:"appId"`
}

type ChunkUploadResponse struct {
	ErrCode      int    `json:"errCode"`
	Offset       int64  `json:"offset"`
	Finished     int    `json:"finished"`
	FinishedFlag string `json:"finishedFlag"`
}
