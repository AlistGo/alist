package http

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/pkg/utils"
)

type SimpleHttp struct {
	client http.Client
}

func (s SimpleHttp) Name() string {
	return "SimpleHttp"
}

func (s SimpleHttp) Items() []model.SettingItem {
	return nil
}

func (s SimpleHttp) Init() (string, error) {
	return "ok", nil
}

func (s SimpleHttp) IsReady() bool {
	return true
}

func (s SimpleHttp) AddURL(args *tool.AddUrlArgs) (string, error) {
	panic("should not be called")
}

func (s SimpleHttp) Remove(task *tool.DownloadTask) error {
	panic("should not be called")
}

func (s SimpleHttp) Status(task *tool.DownloadTask) (*tool.Status, error) {
	panic("should not be called")
}

func (s SimpleHttp) Run(task *tool.DownloadTask) error {
	u := task.Url
	// parse url
	_u, err := url.Parse(u)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(task.Ctx(), http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status code %d", resp.StatusCode)
	}
	// If Path is empty, use Hostname; otherwise, filePath euqals TempDir which causes os.Create to fail
	urlPath := _u.Path
	if urlPath == "" {
		urlPath = strings.ReplaceAll(_u.Host, ".", "_")
	}
	filename := path.Base(urlPath)
	if n, err := parseFilenameFromContentDisposition(resp.Header.Get("Content-Disposition")); err == nil {
		filename = n
	}
	// save to temp dir
	_ = os.MkdirAll(task.TempDir, os.ModePerm)
	filePath := filepath.Join(task.TempDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fileSize := resp.ContentLength
	task.SetTotalBytes(fileSize)
	err = utils.CopyWithCtx(task.Ctx(), file, resp.Body, fileSize, task.SetProgress)
	return err
}

func init() {
	tool.Tools.Add(&SimpleHttp{})
}
