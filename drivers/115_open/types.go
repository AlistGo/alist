package _115_open

import (
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	sdk "github.com/xhofe/115-sdk-go"
)

type Obj sdk.GetFilesResp_File

// Thumb implements model.Thumb.
func (o *Obj) Thumb() string {
	return o.Thumbnail
}

// CreateTime implements model.Obj.
func (o *Obj) CreateTime() time.Time {
	return time.Unix(o.UpPt, 0)
}

// GetHash implements model.Obj.
func (o *Obj) GetHash() utils.HashInfo {
	return utils.NewHashInfo(utils.SHA1, o.Sha1)
}

// GetID implements model.Obj.
func (o *Obj) GetID() string {
	return o.Fid
}

// GetName implements model.Obj.
func (o *Obj) GetName() string {
	return o.Fn
}

// GetPath implements model.Obj.
func (o *Obj) GetPath() string {
	return ""
}

// GetSize implements model.Obj.
func (o *Obj) GetSize() int64 {
	return o.FS
}

// IsDir implements model.Obj.
func (o *Obj) IsDir() bool {
	return o.Fc == "0"
}

// ModTime implements model.Obj.
func (o *Obj) ModTime() time.Time {
	return time.Unix(o.Upt, 0)
}

var _ model.Obj = (*Obj)(nil)
var _ model.Thumb = (*Obj)(nil)
