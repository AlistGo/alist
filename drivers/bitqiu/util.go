package bitqiu

import (
	"path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type Object struct {
	model.Object
	ParentID string
}

func (r Resource) toObject(parentID, parentPath string) (model.Obj, error) {
	id := r.ResourceID
	if id == "" {
		id = r.ResourceUID
	}
	obj := &Object{
		Object: model.Object{
			ID:       id,
			Name:     r.Name,
			IsFolder: r.ResourceType == 1,
		},
		ParentID: parentID,
	}
	if r.Size != nil {
		if size, err := (*r.Size).Int64(); err == nil {
			obj.Size = size
		}
	}
	if ct := parseBitQiuTime(r.CreateTime); !ct.IsZero() {
		obj.Ctime = ct
	}
	if mt := parseBitQiuTime(r.UpdateTime); !mt.IsZero() {
		obj.Modified = mt
	}
	if r.FileMD5 != "" {
		obj.HashInfo = utils.NewHashInfo(utils.MD5, strings.ToLower(r.FileMD5))
	}
	obj.SetPath(path.Join(parentPath, obj.Name))
	return obj, nil
}

func parseBitQiuTime(value *string) time.Time {
	if value == nil {
		return time.Time{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return time.Time{}
	}
	if ts, err := time.ParseInLocation("2006-01-02 15:04:05", trimmed, time.Local); err == nil {
		return ts
	}
	return time.Time{}
}
