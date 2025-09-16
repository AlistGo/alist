package gofile

import "time"

type APIResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

type AccountResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID string `json:"id"`
	} `json:"data"`
}

type AccountInfoResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Email      string `json:"email"`
		RootFolder string `json:"rootFolder"`
	} `json:"data"`
}

type Content struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"` // "file" or "folder"
	Name         string             `json:"name"`
	Size         int64              `json:"size,omitempty"`
	CreateTime   int64              `json:"createTime"`
	ModTime      int64              `json:"modTime,omitempty"`
	DirectLink   string             `json:"directLink,omitempty"`
	Children     map[string]Content `json:"children,omitempty"`
	ParentFolder string             `json:"parentFolder,omitempty"`
	MD5          string             `json:"md5,omitempty"`
	MimeType     string             `json:"mimeType,omitempty"`
	Link         string             `json:"link,omitempty"`
}

type ContentsResponse struct {
	Status string `json:"status"`
	Data   struct {
		IsOwner      bool               `json:"isOwner"`
		ID           string             `json:"id"`
		Type         string             `json:"type"`
		Name         string             `json:"name"`
		ParentFolder string             `json:"parentFolder"`
		CreateTime   int64              `json:"createTime"`
		ChildrenList []string           `json:"childrenList,omitempty"`
		Children     map[string]Content `json:"children,omitempty"`
		Contents     map[string]Content `json:"contents,omitempty"`
		Public       bool               `json:"public,omitempty"`
		Description  string             `json:"description,omitempty"`
		Tags         string             `json:"tags,omitempty"`
		Expiry       int64              `json:"expiry,omitempty"`
	} `json:"data"`
}

type UploadResponse struct {
	Status string `json:"status"`
	Data   struct {
		DownloadPage string `json:"downloadPage"`
		Code         string `json:"code"`
		ParentFolder string `json:"parentFolder"`
		FileId       string `json:"fileId"`
		FileName     string `json:"fileName"`
		GuestToken   string `json:"guestToken,omitempty"`
	} `json:"data"`
}

type DirectLinkResponse struct {
	Status string `json:"status"`
	Data   struct {
		DirectLink string `json:"directLink"`
		ID         string `json:"id"`
	} `json:"data"`
}

type CreateFolderResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID           string `json:"id"`
		Type         string `json:"type"`
		Name         string `json:"name"`
		ParentFolder string `json:"parentFolder"`
		CreateTime   int64  `json:"createTime"`
	} `json:"data"`
}

type CopyResponse struct {
	Status string `json:"status"`
	Data   struct {
		CopiedContents map[string]string `json:"copiedContents"` // oldId -> newId mapping
	} `json:"data"`
}

type UpdateResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"data"`
}

type ErrorResponse struct {
	Status string `json:"status"`
	Error  struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (c *Content) ModifiedTime() time.Time {
	if c.ModTime > 0 {
		return time.Unix(c.ModTime, 0)
	}
	return time.Unix(c.CreateTime, 0)
}

func (c *Content) IsDir() bool {
	return c.Type == "folder"
}
