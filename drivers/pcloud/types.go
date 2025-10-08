package pcloud

import (
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

// ErrorResult represents a pCloud API error response
type ErrorResult struct {
	Result int    `json:"result"`
	Error  string `json:"error"`
}

// TokenResponse represents OAuth token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// ItemResult represents a common pCloud API response
type ItemResult struct {
	Result   int          `json:"result"`
	Metadata *FolderMeta  `json:"metadata,omitempty"`
}

// FolderMeta contains folder metadata including contents
type FolderMeta struct {
	Contents []FileObject `json:"contents,omitempty"`
}

// DownloadLinkResult represents download link response
type DownloadLinkResult struct {
	Result int      `json:"result"`
	Hosts  []string `json:"hosts"`
	Path   string   `json:"path"`
}

// FileObject represents a file or folder object in pCloud
type FileObject struct {
	Name       string    `json:"name"`
	Created    string    `json:"created"`    // pCloud returns RFC1123 format string
	Modified   string    `json:"modified"`   // pCloud returns RFC1123 format string
	IsFolder   bool      `json:"isfolder"`
	FolderID   uint64    `json:"folderid,omitempty"`
	FileID     uint64    `json:"fileid,omitempty"`
	Size       uint64    `json:"size"`
	ParentID   uint64    `json:"parentfolderid"`
	Icon       string    `json:"icon,omitempty"`
	Hash       uint64    `json:"hash,omitempty"`
	Category   int       `json:"category,omitempty"`
	ID         string    `json:"id,omitempty"`
}

// Convert FileObject to model.Obj
func fileToObj(f FileObject) model.Obj {
	// Parse RFC1123 format time from pCloud
	modTime, _ := time.Parse(time.RFC1123, f.Modified)

	obj := model.Object{
		Name:     f.Name,
		Size:     int64(f.Size),
		Modified: modTime,
		IsFolder: f.IsFolder,
	}

	if f.IsFolder {
		obj.ID = "d" + strconv.FormatUint(f.FolderID, 10)
	} else {
		obj.ID = "f" + strconv.FormatUint(f.FileID, 10)
	}

	return &obj
}

// Extract numeric ID from string ID (remove 'd' or 'f' prefix)
func extractID(id string) string {
	if len(id) > 1 && (id[0] == 'd' || id[0] == 'f') {
		return id[1:]
	}
	return id
}

// Get folder ID from path, return "0" for root
func getFolderID(path string) string {
	if path == "/" || path == "" {
		return "0"
	}
	return extractID(path)
}