package wps

import (
	"time"

	"github.com/alist-org/alist/v3/pkg/utils"
)

type workspaceResp struct {
	Companies []struct {
		ID int64 `json:"id"`
	} `json:"companies"`
}

type Group struct {
	CompanyID int64  `json:"company_id"`
	GroupID   int64  `json:"group_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

type groupsResp struct {
	Groups []Group `json:"groups"`
}

type filePerms struct {
	Download int `json:"download"`
}

type FileInfo struct {
	GroupID   int64     `json:"groupid"`
	ParentID  int64     `json:"parentid"`
	Name      string    `json:"fname"`
	Size      int64     `json:"fsize"`
	Type      string    `json:"ftype"`
	Ctime     int64     `json:"ctime"`
	Mtime     int64     `json:"mtime"`
	ID        int64     `json:"id"`
	Deleted   bool      `json:"deleted"`
	FilePerms filePerms `json:"file_perms_acl"`
}

type filesResp struct {
	Files []FileInfo `json:"files"`
}

type downloadResp struct {
	URL    string `json:"url"`
	Result string `json:"result"`
}

type opResp struct {
	Result string `json:"result"`
	Msg    string `json:"msg"`
}

type renameReq struct {
	Fname string `json:"fname"`
}

type mkdirReq struct {
	GroupID  int64  `json:"groupid"`
	Name     string `json:"name"`
	ParentID int64  `json:"parentid"`
}

type moveReq struct {
	FileIDs        []int64 `json:"fileids"`
	TargetGroupID  int64   `json:"target_groupid"`
	TargetParentID int64   `json:"target_parentid"`
}

type deleteReq struct {
	FileIDs []int64 `json:"fileids"`
}

type copyReq struct {
	FileIDs             []int64 `json:"fileids"`
	GroupID             int64   `json:"groupid"`
	TargetGroupID       int64   `json:"target_groupid"`
	TargetParentID      int64   `json:"target_parentid"`
	DuplicatedNameModel int     `json:"duplicated_name_model"`
}

type Obj struct {
	id          string
	name        string
	size        int64
	ctime       time.Time
	mtime       time.Time
	isDir       bool
	hash        utils.HashInfo
	path        string
	canDownload bool
}

func (o *Obj) GetSize() int64 {
	return o.size
}

func (o *Obj) GetName() string {
	return o.name
}

func (o *Obj) ModTime() time.Time {
	return o.mtime
}

func (o *Obj) CreateTime() time.Time {
	return o.ctime
}

func (o *Obj) IsDir() bool {
	return o.isDir
}

func (o *Obj) GetHash() utils.HashInfo {
	return o.hash
}

func (o *Obj) GetID() string {
	return o.id
}

func (o *Obj) GetPath() string {
	return o.path
}
