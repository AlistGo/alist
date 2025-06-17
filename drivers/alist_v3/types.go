package alist_v3

import (
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type ListReq struct {
	model.PageReq
	Path     string `json:"path" form:"path"`
	Password string `json:"password" form:"password"`
	Refresh  bool   `json:"refresh"`
}

type ObjResp struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
	Created  time.Time `json:"created"`
	Sign     string    `json:"sign"`
	Thumb    string    `json:"thumb"`
	Type     int       `json:"type"`
	HashInfo string    `json:"hashinfo"`
}

type FsListResp struct {
	Content  []ObjResp `json:"content"`
	Total    int64     `json:"total"`
	Readme   string    `json:"readme"`
	Write    bool      `json:"write"`
	Provider string    `json:"provider"`
}

type FsGetReq struct {
	Path     string `json:"path" form:"path"`
	Password string `json:"password" form:"password"`
}

type FsGetResp struct {
	ObjResp
	RawURL   string    `json:"raw_url"`
	Readme   string    `json:"readme"`
	Provider string    `json:"provider"`
	Related  []ObjResp `json:"related"`
}

type MkdirOrLinkReq struct {
	Path string `json:"path" form:"path"`
}

type MoveCopyReq struct {
	SrcDir string   `json:"src_dir"`
	DstDir string   `json:"dst_dir"`
	Names  []string `json:"names"`
}

type RenameReq struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type RemoveReq struct {
	Dir   string   `json:"dir"`
	Names []string `json:"names"`
}

type LoginResp struct {
	Token string `json:"token"`
}

type MeResp struct {
	Id         int    `json:"id"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	BasePath   string `json:"base_path"`
	Role       int    `json:"role"`
	Disabled   bool   `json:"disabled"`
	Permission int    `json:"permission"`
	SsoId      string `json:"sso_id"`
	Otp        bool   `json:"otp"`
}

type ArchiveMetaReq struct {
	ArchivePass string `json:"archive_pass"`
	Password    string `json:"password"`
	Path        string `json:"path"`
	Refresh     bool   `json:"refresh"`
}

type TreeResp struct {
	ObjResp
	Children  []TreeResp `json:"children"`
	hashCache *utils.HashInfo
}

func (t *TreeResp) GetSize() int64 {
	return t.Size
}

func (t *TreeResp) GetName() string {
	return t.Name
}

func (t *TreeResp) ModTime() time.Time {
	return t.Modified
}

func (t *TreeResp) CreateTime() time.Time {
	return t.Created
}

func (t *TreeResp) IsDir() bool {
	return t.ObjResp.IsDir
}

func (t *TreeResp) GetHash() utils.HashInfo {
	return utils.FromString(t.HashInfo)
}

func (t *TreeResp) GetID() string {
	return ""
}

func (t *TreeResp) GetPath() string {
	return ""
}

func (t *TreeResp) GetChildren() []model.ObjTree {
	ret := make([]model.ObjTree, 0, len(t.Children))
	for _, child := range t.Children {
		ret = append(ret, &child)
	}
	return ret
}

func (t *TreeResp) Thumb() string {
	return t.ObjResp.Thumb
}

type ArchiveMetaResp struct {
	Comment   string     `json:"comment"`
	Encrypted bool       `json:"encrypted"`
	Content   []TreeResp `json:"content"`
	RawURL    string     `json:"raw_url"`
	Sign      string     `json:"sign"`
}

type ArchiveListReq struct {
	model.PageReq
	ArchiveMetaReq
	InnerPath string `json:"inner_path"`
}

type ArchiveListResp struct {
	Content []ObjResp `json:"content"`
	Total   int64     `json:"total"`
}

type DecompressReq struct {
	ArchivePass   string   `json:"archive_pass"`
	CacheFull     bool     `json:"cache_full"`
	DstDir        string   `json:"dst_dir"`
	InnerPath     string   `json:"inner_path"`
	Name          []string `json:"name"`
	PutIntoNewDir bool     `json:"put_into_new_dir"`
	SrcDir        string   `json:"src_dir"`
}
