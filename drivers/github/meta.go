package github

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	driver.RootPath
	Token            string `json:"token" type:"string" required:"true"`
	Owner            string `json:"owner" type:"string" required:"true"`
	Repo             string `json:"repo" type:"string" required:"true"`
	Ref              string `json:"ref" type:"string" help:"A branch, a tag or a commit SHA, main branch by default."`
	GitHubProxy      string `json:"gh_proxy" type:"string" help:"GitHub proxy, e.g. https://ghproxy.net/raw.githubusercontent.com or https://gh-proxy.com/raw.githubusercontent.com"`
	GPGPrivateKey    string `json:"gpg_private_key" type:"text"`
	GPGKeyPassphrase string `json:"gpg_key_passphrase" type:"string"`
	CommitterName    string `json:"committer_name" type:"string"`
	CommitterEmail   string `json:"committer_email" type:"string"`
	AuthorName       string `json:"author_name" type:"string"`
	AuthorEmail      string `json:"author_email" type:"string"`
	MkdirCommitMsg   string `json:"mkdir_commit_message" type:"text" default:"{{.UserName}} mkdir {{.ObjPath}}"`
	DeleteCommitMsg  string `json:"delete_commit_message" type:"text" default:"{{.UserName}} remove {{.ObjPath}}"`
	PutCommitMsg     string `json:"put_commit_message" type:"text" default:"{{.UserName}} upload {{.ObjPath}}"`
	RenameCommitMsg  string `json:"rename_commit_message" type:"text" default:"{{.UserName}} rename {{.ObjPath}} to {{.TargetName}}"`
	CopyCommitMsg    string `json:"copy_commit_message" type:"text" default:"{{.UserName}} copy {{.ObjPath}} to {{.TargetPath}}"`
	MoveCommitMsg    string `json:"move_commit_message" type:"text" default:"{{.UserName}} move {{.ObjPath}} to {{.TargetPath}}"`
}

var config = driver.Config{
	Name:        "GitHub API",
	LocalSort:   true,
	DefaultRoot: "/",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &Github{}
	})
}
