package _115

import (
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

var _ model.Obj = (*FileObj)(nil)

type FileObj struct {
	driver.File
}

func (f *FileObj) CreateTime() time.Time {
	return f.File.CreateTime
}

func (f *FileObj) GetHash() utils.HashInfo {
	return utils.NewHashInfo(utils.SHA1, f.Sha1)
}

type UploadResult struct {
	driver.BasicResp
	Data struct {
		PickCode string `json:"pick_code"`
		FileSize int    `json:"file_size"`
		FileID   string `json:"file_id"`
		ThumbURL string `json:"thumb_url"`
		Sha1     string `json:"sha1"`
		Aid      int    `json:"aid"`
		FileName string `json:"file_name"`
		Cid      string `json:"cid"`
		IsVideo  int    `json:"is_video"`
	} `json:"data"`
}
