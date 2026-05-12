package handles

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type larkExportDownloader interface {
	DownloadExportFile(ctx context.Context, fileToken string) (io.Reader, string, error)
}

func LarkExportDownload(c *gin.Context) {
	rawPath := c.Query("path")
	fileToken := c.Query("file_token")
	password := c.Query("password")
	filename := strings.TrimSpace(c.Query("filename"))
	if rawPath == "" || fileToken == "" {
		common.ErrorStrResp(c, "path and file_token are required", 400)
		return
	}

	user := c.MustGet("user").(*model.User)
	reqPath, err := user.JoinPath(rawPath)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500)
			return
		}
	}
	if !common.CanAccessWithRoles(user, meta, reqPath, password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}

	storage, err := fs.GetStorage(reqPath, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	downloader, ok := storage.(larkExportDownloader)
	if !ok || storage.GetStorage().Driver != "Lark" {
		common.ErrorStrResp(c, "lark export download is not supported for this storage", 400)
		return
	}

	reader, respFilename, err := downloader.DownloadExportFile(c, fileToken)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if filename == "" {
		filename = respFilename
	}
	if filename == "" {
		filename = stdpath.Base(reqPath)
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename)))
	c.Header("Content-Type", utils.GetMimeType(filename))
	c.Status(http.StatusOK)
	if _, err = io.Copy(c.Writer, reader); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
}
