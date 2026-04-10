package handles

import (
	stderrors "errors"
	"net/url"
	stdpath "path"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

type FsChunkInitReq struct {
	Size         int64  `json:"size" binding:"required"`
	AsTask       bool   `json:"as_task"`
	Overwrite    *bool  `json:"overwrite"`
	LastModified int64  `json:"last_modified"`
	Mimetype     string `json:"mimetype"`
}

type FsChunkCompleteReq struct {
	UploadID string `json:"upload_id" binding:"required"`
}

type FsChunkCancelReq struct {
	UploadID string `json:"upload_id" binding:"required"`
}

func FsUploadPolicy(c *gin.Context) {
	rawPath := c.GetHeader("File-Path")
	password := c.GetHeader("Password")
	path, err, code := resolveUploadPathAndCheckPermission(c, rawPath, password)
	if err != nil {
		common.ErrorResp(c, err, code)
		return
	}
	policy, err := fs.GetUploadPolicy(path)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, gin.H{
		"provider":      policy.Provider,
		"chunk_size_mb": policy.ChunkSizeMB,
		"enabled":       policy.ChunkSizeMB > 0,
	})
}

func FsChunkInit(c *gin.Context) {
	var req FsChunkInitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if req.Size <= 0 {
		common.ErrorResp(c, fs.ErrChunkUploadBadRequest, 400)
		return
	}

	rawPath := c.GetHeader("File-Path")
	password := c.GetHeader("Password")
	path, err, code := resolveUploadPathAndCheckPermission(c, rawPath, password)
	if err != nil {
		common.ErrorResp(c, err, code)
		return
	}

	asTask := req.AsTask
	if asTaskHeader := c.GetHeader("As-Task"); asTaskHeader != "" {
		asTask = asTaskHeader == "true"
	}
	overwrite := true
	if req.Overwrite != nil {
		overwrite = *req.Overwrite
	}
	if overwriteHeader := c.GetHeader("Overwrite"); overwriteHeader != "" {
		overwrite = overwriteHeader != "false"
	}
	lastModified := getLastModified(c)
	if req.LastModified > 0 {
		lastModified = time.UnixMilli(req.LastModified)
	}

	mimetype := c.GetHeader("Content-Type")
	if mimetype == "" {
		mimetype = req.Mimetype
	}
	if mimetype == "" {
		_, fileName := stdpath.Split(path)
		mimetype = utils.GetMimeType(fileName)
	}

	hashes := make(map[*utils.HashType]string)
	if md5 := c.GetHeader("X-File-Md5"); md5 != "" {
		hashes[utils.MD5] = md5
	}
	if sha1 := c.GetHeader("X-File-Sha1"); sha1 != "" {
		hashes[utils.SHA1] = sha1
	}
	if sha256 := c.GetHeader("X-File-Sha256"); sha256 != "" {
		hashes[utils.SHA256] = sha256
	}

	user := c.MustGet("user").(*model.User)
	resp, err := fs.InitChunkUpload(c, fs.ChunkUploadInitArgs{
		UserID:       user.ID,
		Path:         path,
		Size:         req.Size,
		AsTask:       asTask,
		Overwrite:    overwrite,
		LastModified: lastModified,
		Mimetype:     mimetype,
		Hashes:       hashes,
	})
	if err != nil {
		respondChunkUploadErr(c, err)
		return
	}

	common.SuccessResp(c, gin.H{
		"upload_id":        resp.UploadID,
		"provider":         resp.Provider,
		"chunk_size_mb":    resp.ChunkSizeMB,
		"chunk_size_bytes": resp.ChunkSizeBytes,
	})
}

func FsChunkUpload(c *gin.Context) {
	uploadID := c.GetHeader("Upload-Id")
	if uploadID == "" {
		uploadID = c.Query("upload_id")
	}
	if uploadID == "" {
		common.ErrorResp(c, fs.ErrChunkUploadBadRequest, 400)
		return
	}

	chunkIndexRaw := c.GetHeader("Chunk-Index")
	if chunkIndexRaw == "" {
		chunkIndexRaw = c.Query("chunk_index")
	}
	chunkIndex, err := strconv.ParseInt(chunkIndexRaw, 10, 64)
	if err != nil || chunkIndex < 0 {
		common.ErrorResp(c, fs.ErrChunkUploadBadRequest, 400)
		return
	}

	user := c.MustGet("user").(*model.User)
	defer c.Request.Body.Close()
	uploaded, total, err := fs.AppendChunk(c, uploadID, user.ID, chunkIndex, c.Request.Body)
	if err != nil {
		respondChunkUploadErr(c, err)
		return
	}
	common.SuccessResp(c, gin.H{
		"uploaded_size": uploaded,
		"total_size":    total,
	})
}

func FsChunkComplete(c *gin.Context) {
	var req FsChunkCompleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	t, err := fs.CompleteChunkUpload(c, req.UploadID, user.ID)
	if err != nil {
		respondChunkUploadErr(c, err)
		return
	}
	if t == nil {
		common.SuccessResp(c)
		return
	}
	common.SuccessResp(c, gin.H{
		"task": getTaskInfo(t),
	})
}

func FsChunkCancel(c *gin.Context) {
	var req FsChunkCancelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	err := fs.CancelChunkUpload(req.UploadID, user.ID)
	if err != nil {
		respondChunkUploadErr(c, err)
		return
	}
	common.SuccessResp(c)
}

func respondChunkUploadErr(c *gin.Context, err error) {
	code := 500
	switch {
	case stderrors.Is(err, fs.ErrChunkUploadBadRequest):
		code = 400
	case stderrors.Is(err, fs.ErrChunkUploadNotFound):
		code = 404
	case stderrors.Is(err, fs.ErrChunkUploadNotOwner):
		code = 403
	case stderrors.Is(err, fs.ErrChunkUploadDisabled):
		code = 405
	case stderrors.Is(err, fs.ErrChunkUploadNeedRetry):
		code = 409
	case stderrors.Is(err, fs.ErrChunkUploadFileExists):
		code = 403
	}
	common.ErrorResp(c, err, code, code >= 500)
}

func resolveUploadPathAndCheckPermission(c *gin.Context, rawPath, password string) (string, error, int) {
	if rawPath == "" {
		return "", fs.ErrChunkUploadBadRequest, 400
	}
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", err, 400
	}
	user := c.MustGet("user").(*model.User)
	path, err = user.JoinPath(path)
	if err != nil {
		return "", err, 403
	}
	meta, err := op.GetNearestMeta(stdpath.Dir(path))
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			return "", err, 500
		}
	}
	perm := common.MergeRolePermissions(user, path)
	if !(common.CanAccessWithRoles(user, meta, path, password) &&
		(common.HasPermission(perm, common.PermWrite) || common.CanWrite(meta, stdpath.Dir(path)))) {
		return "", errs.PermissionDenied, 403
	}
	return path, nil, 0
}
