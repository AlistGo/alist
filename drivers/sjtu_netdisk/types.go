package sjtu_netdisk

import (
	"time"
)

// for refreshToken
type TokenResp struct {
	LibraryId   string `json:"libraryId"`
	SpaceId     string `json:"spaceId"`
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
}

// for List
type FolderListResp struct {
	Path        []string     `json:"path"`
	SubDirCount int          `json:"subDirCount"`
	FileCount   int          `json:"fileCount"`
	TotalNum    int          `json:"totalNum"`
	Contents    []NetdiskObj `json:"contents"`
}

// for "contents" embedded in FolderListResp
type NetdiskObj struct {
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Size             string    `json:"size"`
	ModificationTime time.Time `json:"modificationTime"`
	CreationTime     time.Time `json:"creationTime"`
	ContentType      string    `json:"contentType"`
}

// for step 1 of Put (apply for confirm key)
type UploadInitResp struct {
	Domain     string            `json:"domain"`  // domain
	Path       string            `json:"path"`    // hashed path
	Headers    map[string]string `json:"headers"` // AWS signature request headers
	ConfirmKey string            `json:"confirmKey"`
	Expiration string            `json:"expiration"`
}

// for step 3 of Put (confirm the upload)
type ConfirmResp struct {
	Path          []string `json:"path"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Size          string   `json:"size"`
	IsOverwritten bool     `json:"isOverwritten"`
}

// for Move, Copy, Rename
type MoveCopyResp struct {
	Path []string `json:"path"` // path is divided and the last one is name
}

// for folder Copy, Move with task (no conflict)
type TaskInitResp struct {
	TaskId int64 `json:"taskId"`
}

// for the result of polling task status
type TaskStatusItem struct {
	TaskId int64         `json:"taskId"`
	Status int           `json:"status"` // 204=complete without conflict
	Result *MoveCopyResp `json:"result,omitempty"`
}
