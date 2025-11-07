package gitee

import (
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

type Links struct {
	Self string `json:"self"`
	Html string `json:"html"`
}

type Content struct {
	Type        string `json:"type"`
	Size        *int64 `json:"size"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Sha         string `json:"sha"`
	URL         string `json:"url"`
	HtmlURL     string `json:"html_url"`
	DownloadURL string `json:"download_url"`
	Links       Links  `json:"_links"`
}

func (c Content) toModelObj() model.Obj {
	size := int64(0)
	if c.Size != nil {
		size = *c.Size
	}
	return &Object{
		Object: model.Object{
			ID:       c.Path,
			Name:     c.Name,
			Size:     size,
			Modified: time.Unix(0, 0),
			IsFolder: c.Type == "dir",
		},
		DownloadURL: c.DownloadURL,
		HtmlURL:     c.HtmlURL,
	}
}

type Object struct {
	model.Object
	DownloadURL string
	HtmlURL     string
}

func (o *Object) URL() string {
	return o.DownloadURL
}

type Repo struct {
	DefaultBranch string `json:"default_branch"`
}

type ErrResp struct {
	Message string `json:"message"`
}
