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

type CreateDirData struct {
	DirID    string `json:"dirId"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
}
