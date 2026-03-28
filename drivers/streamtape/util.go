package streamtape

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
)

const apiBase = "https://api.streamtape.com"

func (d *Streamtape) callAPI(ctx context.Context, endpoint string, params map[string]string, out any) error {
	query := map[string]string{
		"login": d.APILogin,
		"key":   d.APIKey,
	}
	for k, v := range params {
		if strings.TrimSpace(v) == "" {
			continue
		}
		query[k] = v
	}

	var resp apiResponse
	r, err := base.RestyClient.R().
		SetContext(ctx).
		SetQueryParams(query).
		SetResult(&resp).
		Get(apiBase + endpoint)
	if err != nil {
		return err
	}
	if r.StatusCode() != http.StatusOK {
		return fmt.Errorf("streamtape http error: %d", r.StatusCode())
	}
	if resp.Status != 200 {
		return fmt.Errorf("streamtape api error: status=%d msg=%s", resp.Status, resp.Msg)
	}
	if out == nil || len(resp.Result) == 0 || string(resp.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("decode streamtape result failed: %w", err)
	}
	return nil
}

func folderIDFromObjID(id string) string {
	if id == "" || id == "0" || id == "/" {
		return "0"
	}
	if strings.HasPrefix(id, "d:") {
		return strings.TrimPrefix(id, "d:")
	}
	return id
}

func fileIDFromObjID(id string) string {
	if strings.HasPrefix(id, "f:") {
		return strings.TrimPrefix(id, "f:")
	}
	return id
}

func encodeFolderID(id string) string {
	if id == "" || id == "0" || id == "/" {
		return "d:0"
	}
	return "d:" + id
}

func encodeFileID(id string) string {
	if strings.HasPrefix(id, "f:") {
		return id
	}
	return "f:" + id
}

func extractFileIDFromLink(link string) string {
	if link == "" {
		return ""
	}
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "v" {
			return parts[i+1]
		}
	}
	return ""
}

func buildFileObj(f fileItem) model.Obj {
	id := f.LinkID
	if id == "" {
		id = extractFileIDFromLink(f.Link)
	}
	mod := time.Now()
	if f.CreatedAt > 0 {
		mod = time.Unix(f.CreatedAt, 0)
	}
	return &model.Object{
		ID:       encodeFileID(id),
		Name:     f.Name,
		Size:     f.Size,
		Modified: mod,
		IsFolder: false,
	}
}

func extractFileIDFromUploadBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	if resp.Status != 200 || len(resp.Result) == 0 {
		return ""
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return ""
	}
	for _, key := range []string{"file", "fileid", "id", "linkid"} {
		if v, ok := result[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

const remoteUploadPrefix = "ru:"

func encodeRemoteUploadID(id string) string {
	return remoteUploadPrefix + id
}

func remoteUploadIDFromObjID(id string) string {
	if strings.HasPrefix(id, remoteUploadPrefix) {
		return strings.TrimPrefix(id, remoteUploadPrefix)
	}
	return ""
}

func isRemoteUploadID(id string) bool {
	return strings.HasPrefix(id, remoteUploadPrefix)
}
