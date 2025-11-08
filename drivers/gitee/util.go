package gitee

import (
	"fmt"
	"net/url"

	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

func (d *Gitee) getRepo() (*Repo, error) {
	req := d.client.R()
	if d.Token != "" {
		req.SetQueryParam("access_token", d.Token)
	}
	if d.Cookie != "" {
		req.SetHeader("Cookie", d.Cookie)
	}
	escapedOwner := url.PathEscape(d.Owner)
	escapedRepo := url.PathEscape(d.Repo)
	res, err := req.Get(fmt.Sprintf("/repos/%s/%s", escapedOwner, escapedRepo))
	if err != nil {
		return nil, err
	}
	if res.IsError() {
		return nil, toErr(res)
	}
	var repo Repo
	if err := utils.Json.Unmarshal(res.Body(), &repo); err != nil {
		return nil, err
	}
	if repo.DefaultBranch == "" {
		return nil, fmt.Errorf("failed to fetch default branch")
	}
	return &repo, nil
}

func toErr(res *resty.Response) error {
	var errMsg ErrResp
	if err := utils.Json.Unmarshal(res.Body(), &errMsg); err == nil && errMsg.Message != "" {
		return fmt.Errorf("%s: %s", res.Status(), errMsg.Message)
	}
	return fmt.Errorf(res.Status())
}
