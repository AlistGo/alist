package _123Open

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/http"
)

const (
	// baseurl
	ApiBaseURL = "https://open-api.123pan.com"

	// auth
	ApiToken = "/api/v1/access_token"

	// file list
	ApiFileList = "/api/v2/file/list"

	// direct link
	ApiGetDirectLink = "/api/v1/direct-link/url"

	// mkdir
	ApiMakeDir = "/upload/v1/file/mkdir"

	// remove
	ApiRemove = "/api/v1/file/trash"

	// upload
	ApiUploadDomainURL   = "/upload/v2/file/domain"
	ApiSingleUploadURL   = "/upload/v2/file/single/create"
	ApiCreateUploadURL   = "/upload/v2/file/create"
	ApiUploadSliceURL    = "/upload/v2/file/slice"
	ApiUploadCompleteURL = "/upload/v2/file/upload_complete"

	// move
	ApiMove = "/api/v1/file/move"

	// rename
	ApiRename = "/api/v1/file/name"
)

type Response[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type TokenResp struct {
	Code    int       `json:"code"`
	Message string    `json:"message"`
	Data    TokenData `json:"data"`
}

type TokenData struct {
	AccessToken string `json:"accessToken"`
	ExpiredAt   string `json:"expiredAt"`
}

type FileListResp struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    FileListData `json:"data"`
}

type FileListData struct {
	LastFileId int64  `json:"lastFileId"`
	FileList   []File `json:"fileList"`
}

type DirectLinkResp struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    DirectLinkData `json:"data"`
}

type DirectLinkData struct {
	URL string `json:"url"`
}

type MakeDirRequest struct {
	Name     string `json:"name"`
	ParentID int64  `json:"parentID"`
}

type MakeDirResp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    MakeDirData `json:"data"`
}

type MakeDirData struct {
	DirID int64 `json:"dirID"`
}

type RemoveRequest struct {
	FileIDs []int64 `json:"fileIDs"`
}

type UploadCreateResp struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    UploadCreateData `json:"data"`
}

type UploadCreateData struct {
	FileID      int64    `json:"fileId"`
	Reuse       bool     `json:"reuse"`
	PreuploadID string   `json:"preuploadId"`
	SliceSize   int64    `json:"sliceSize"`
	Servers     []string `json:"servers"`
}

type UploadUrlResp struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    UploadUrlData `json:"data"`
}

type UploadUrlData struct {
	PresignedURL string `json:"presignedUrl"`
}

type UploadCompleteResp struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    UploadCompleteData `json:"data"`
}

type UploadCompleteData struct {
	FileID    int  `json:"fileID"`
	Completed bool `json:"completed"`
}

func (d *Open123) Request(endpoint string, method string, setup func(*resty.Request), result any) (*resty.Response, error) {
	client := resty.New()
	token, err := d.tm.getToken()
	if err != nil {
		return nil, err
	}

	req := client.R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Platform", "open_platform").
		SetHeader("Content-Type", "application/json").
		SetResult(result)

	if setup != nil {
		setup(req)
	}

	switch method {
	case http.MethodGet:
		return req.Get(ApiBaseURL + endpoint)
	case http.MethodPost:
		return req.Post(ApiBaseURL + endpoint)
	case http.MethodPut:
		return req.Put(ApiBaseURL + endpoint)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (d *Open123) RequestTo(fullURL string, method string, setup func(*resty.Request), result any) (*resty.Response, error) {
	client := resty.New()

	token, err := d.tm.getToken()
	if err != nil {
		return nil, err
	}

	req := client.R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Platform", "open_platform").
		SetHeader("Content-Type", "application/json").
		SetResult(result)

	if setup != nil {
		setup(req)
	}

	switch method {
	case http.MethodGet:
		return req.Get(fullURL)
	case http.MethodPost:
		return req.Post(fullURL)
	case http.MethodPut:
		return req.Put(fullURL)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}
