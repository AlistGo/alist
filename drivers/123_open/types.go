package _123Open

import (
	"fmt"
	"github.com/alist-org/alist/v3/pkg/utils"
	"time"
)

type File struct {
	FileName     string `json:"filename"`
	Size         int64  `json:"size"`
	CreateAt     string `json:"createAt"`
	UpdateAt     string `json:"updateAt"`
	FileId       int64  `json:"fileId"`
	Type         int    `json:"type"`
	Etag         string `json:"etag"`
	S3KeyFlag    string `json:"s3KeyFlag"`
	ParentFileId int    `json:"parentFileId"`
	Category     int    `json:"category"`
	Status       int    `json:"status"`
	Trashed      int    `json:"trashed"`
}

func (f File) GetID() string {
	return fmt.Sprint(f.FileId)
}

func (f File) GetName() string {
	return f.FileName
}

func (f File) GetSize() int64 {
	return f.Size
}

func (f File) IsDir() bool {
	return f.Type == 1
}

func (f File) GetModified() string {
	return f.UpdateAt
}

func (f File) GetThumb() string {
	return ""
}

func (f File) ModTime() time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", f.UpdateAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (f File) CreateTime() time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", f.CreateAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (f File) GetHash() utils.HashInfo {
	return utils.NewHashInfo(utils.MD5, f.Etag)
}

func (f File) GetPath() string {
	return ""
}
