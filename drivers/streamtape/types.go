package streamtape

import "encoding/json"

type apiResponse struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Result json.RawMessage `json:"result"`
}

type accountInfo struct {
	APIID    string `json:"apiid"`
	Email    string `json:"email"`
	SignupAt string `json:"signup_at"`
}

type listFolderResult struct {
	Folders []folderItem `json:"folders"`
	Files   []fileItem   `json:"files"`
}

type folderItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fileItem struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Link      string `json:"link"`
	CreatedAt int64  `json:"created_at"`
	Downloads int64  `json:"downloads"`
	LinkID    string `json:"linkid"`
	Convert   string `json:"convert"`
}

type dlTicketResult struct {
	Ticket   string `json:"ticket"`
	WaitTime int    `json:"wait_time"`
}

type dlResult struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type createFolderResult struct {
	FolderID string `json:"folderid"`
}

type uploadURLResult struct {
	URL string `json:"url"`
}

type remoteDlAddResult struct {
	ID       string `json:"id"`
	FolderID string `json:"folderid"`
}

type remoteDlStatusResult map[string]remoteDlStatusItem

type remoteDlStatusItem struct {
	ID         string      `json:"id"`
	RemoteURL  string      `json:"remoteurl"`
	Status     string      `json:"status"`
	BytesLoaded interface{} `json:"bytes_loaded"`
	BytesTotal interface{} `json:"bytes_total"`
	FolderID   string      `json:"folderid"`
	Added      string      `json:"added"`
	LastUpdate string      `json:"last_update"`
	ExtID      bool        `json:"extid"`
	URL        bool        `json:"url"`
}

type fileInfoResult map[string]fileInfoItem

type fileInfoItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Type      string `json:"type"`
	Converted bool   `json:"converted"`
	Status    int    `json:"status"`
}

type conversionResult []conversionItem

type conversionItem struct {
	Name     string `json:"name"`
	FolderID string `json:"folderid"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Retries  int    `json:"retries"`
	Link     string `json:"link"`
	LinkID   string `json:"linkid"`
}
