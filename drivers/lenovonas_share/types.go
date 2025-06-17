package LenovoNasShare

import (
	"encoding/json"
	"time"

	"github.com/alist-org/alist/v3/pkg/utils"

	_ "github.com/alist-org/alist/v3/internal/model"
)

func (f *File) UnmarshalJSON(data []byte) error {
	type Alias File
	aux := &struct {
		CreateAt int64 `json:"time"`
		UpdateAt int64 `json:"chtime"`
		*Alias
	}{
		Alias: (*Alias)(f),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	f.CreateAt = time.Unix(aux.CreateAt, 0)
	f.UpdateAt = time.Unix(aux.UpdateAt, 0)

	return nil
}

type File struct {
	FileName string    `json:"name"`
	Size     int64     `json:"size"`
	CreateAt time.Time `json:"time"`
	UpdateAt time.Time `json:"chtime"`
	Path     string    `json:"path"`
	Type     string    `json:"type"`
}

func (f File) GetHash() utils.HashInfo {
	return utils.HashInfo{}
}

func (f File) GetPath() string {
	return f.Path
}

func (f File) GetSize() int64 {
	if f.IsDir() {
		return 0
	} else {
		return f.Size
	}
}

func (f File) GetName() string {
	return f.FileName
}

func (f File) ModTime() time.Time {
	return f.UpdateAt
}

func (f File) CreateTime() time.Time {
	return f.CreateAt
}

func (f File) IsDir() bool {
	return f.Type == "dir"
}

func (f File) GetID() string {
	return f.GetPath()
}

type Files struct {
	Data struct {
		List    []File `json:"list"`
		HasMore bool   `json:"has_more"`
	} `json:"data"`
}
