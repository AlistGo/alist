package github

import (
	"github.com/alist-org/alist/v3/internal/model"
	"time"
)

type Links struct {
	Git  string `json:"git"`
	Html string `json:"html"`
	Self string `json:"self"`
}

type Object struct {
	Type            string   `json:"type"`
	Encoding        string   `json:"encoding" required:"false"`
	Size            int64    `json:"size"`
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Content         string   `json:"Content" required:"false"`
	Sha             string   `json:"sha"`
	URL             string   `json:"url"`
	GitURL          string   `json:"git_url"`
	HtmlURL         string   `json:"html_url"`
	DownloadURL     string   `json:"download_url"`
	Entries         []Object `json:"entries" required:"false"`
	Links           Links    `json:"_links"`
	SubmoduleGitURL string   `json:"submodule_git_url" required:"false"`
	Target          string   `json:"target" required:"false"`
}

func (o *Object) toModelObj() *model.Object {
	return &model.Object{
		Name:     o.Name,
		Size:     o.Size,
		Modified: time.Unix(0, 0),
		IsFolder: o.Type == "dir",
	}
}

type PutBlobResp struct {
	URL string `json:"url"`
	Sha string `json:"sha"`
}

type ErrResp struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
	Status           string `json:"status"`
}

type TreeObjReq struct {
	Path string      `json:"path"`
	Mode string      `json:"mode"`
	Type string      `json:"type"`
	Sha  interface{} `json:"sha"`
}

type TreeObjResp struct {
	TreeObjReq
	Size int64  `json:"size" required:"false"`
	URL  string `json:"url"`
}

func (o *TreeObjResp) toModelObj() *model.Object {
	return &model.Object{
		Name:     o.Path,
		Size:     o.Size,
		Modified: time.Unix(0, 0),
		IsFolder: o.Type == "tree",
	}
}

type TreeResp struct {
	Sha       string        `json:"sha"`
	URL       string        `json:"url"`
	Trees     []TreeObjResp `json:"tree"`
	Truncated bool          `json:"truncated"`
}

type TreeReq struct {
	BaseTree interface{}   `json:"base_tree,omitempty"`
	Trees    []interface{} `json:"tree"`
}

type CommitResp struct {
	Sha string `json:"sha"`
}

type BranchResp struct {
	Name   string     `json:"name"`
	Commit CommitResp `json:"commit"`
}

type UpdateRefReq struct {
	Sha   string `json:"sha"`
	Force bool   `json:"force"`
}

type RepoResp struct {
	DefaultBranch string `json:"default_branch"`
}

type UserResp struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
