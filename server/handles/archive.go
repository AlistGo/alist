package handles

import (
	"fmt"
	"net/url"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ArchiveMetaReq struct {
	Path        string `json:"path" form:"path"`
	Password    string `json:"password" form:"password"`
	Refresh     bool   `json:"refresh" form:"refresh"`
	ArchivePass string `json:"archive_pass" form:"archive_pass"`
}

type ArchiveMetaResp struct {
	Comment     string               `json:"comment"`
	IsEncrypted bool                 `json:"encrypted"`
	Content     []ArchiveContentResp `json:"content"`
	Sort        *model.Sort          `json:"sort,omitempty"`
	RawURL      string               `json:"raw_url"`
	Sign        string               `json:"sign"`
}

type ArchiveContentResp struct {
	ObjResp
	Children []ArchiveContentResp `json:"children"`
}

func toObjsRespWithoutSignAndThumb(obj model.Obj) ObjResp {
	return ObjResp{
		Name:        obj.GetName(),
		Size:        obj.GetSize(),
		IsDir:       obj.IsDir(),
		Modified:    obj.ModTime(),
		Created:     obj.CreateTime(),
		HashInfoStr: obj.GetHash().String(),
		HashInfo:    obj.GetHash().Export(),
		Sign:        "",
		Thumb:       "",
		Type:        utils.GetObjType(obj.GetName(), obj.IsDir()),
	}
}

func toContentResp(objs []model.ObjTree) []ArchiveContentResp {
	if objs == nil {
		return nil
	}
	ret, _ := utils.SliceConvert(objs, func(src model.ObjTree) (ArchiveContentResp, error) {
		return ArchiveContentResp{
			ObjResp:  toObjsRespWithoutSignAndThumb(src),
			Children: toContentResp(src.GetChildren()),
		}, nil
	})
	return ret
}

func FsArchiveMeta(c *gin.Context) {
	var req ArchiveMetaReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanReadArchives() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)
	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}
	archiveArgs := model.ArchiveArgs{
		LinkArgs: model.LinkArgs{
			Header:  c.Request.Header,
			Type:    c.Query("type"),
			HttpReq: c.Request,
		},
		Password: req.ArchivePass,
	}
	ret, err := fs.ArchiveMeta(c, reqPath, model.ArchiveMetaArgs{
		ArchiveArgs: archiveArgs,
		Refresh:     req.Refresh,
	})
	if err != nil {
		if errors.Is(err, errs.WrongArchivePassword) {
			common.ErrorResp(c, err, 202)
		} else {
			common.ErrorResp(c, err, 500)
		}
		return
	}
	s := ""
	if isEncrypt(meta, reqPath) || setting.GetBool(conf.SignAll) {
		s = sign.SignArchive(reqPath)
	}
	api := "/ae"
	if ret.DriverProviding {
		api = "/ad"
	}
	common.SuccessResp(c, ArchiveMetaResp{
		Comment:     ret.GetComment(),
		IsEncrypted: ret.IsEncrypted(),
		Content:     toContentResp(ret.GetTree()),
		Sort:        ret.Sort,
		RawURL:      fmt.Sprintf("%s%s%s", common.GetApiUrl(c.Request), api, utils.EncodePath(reqPath, true)),
		Sign:        s,
	})
}

type ArchiveListReq struct {
	ArchiveMetaReq
	model.PageReq
	InnerPath string `json:"inner_path" form:"inner_path"`
}

type ArchiveListResp struct {
	Content []ObjResp `json:"content"`
	Total   int64     `json:"total"`
}

func FsArchiveList(c *gin.Context) {
	var req ArchiveListReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	user := c.MustGet("user").(*model.User)
	if !user.CanReadArchives() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	meta, err := op.GetNearestMeta(reqPath)
	if err != nil {
		if !errors.Is(errors.Cause(err), errs.MetaNotFound) {
			common.ErrorResp(c, err, 500, true)
			return
		}
	}
	c.Set("meta", meta)
	if !common.CanAccess(user, meta, reqPath, req.Password) {
		common.ErrorStrResp(c, "password is incorrect or you have no permission", 403)
		return
	}
	objs, err := fs.ArchiveList(c, reqPath, model.ArchiveListArgs{
		ArchiveInnerArgs: model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					Header:  c.Request.Header,
					Type:    c.Query("type"),
					HttpReq: c.Request,
				},
				Password: req.ArchivePass,
			},
			InnerPath: utils.FixAndCleanPath(req.InnerPath),
		},
		Refresh: req.Refresh,
	})
	if err != nil {
		if errors.Is(err, errs.WrongArchivePassword) {
			common.ErrorResp(c, err, 202)
		} else {
			common.ErrorResp(c, err, 500)
		}
		return
	}
	total, objs := pagination(objs, &req.PageReq)
	ret, _ := utils.SliceConvert(objs, func(src model.Obj) (ObjResp, error) {
		return toObjsRespWithoutSignAndThumb(src), nil
	})
	common.SuccessResp(c, ArchiveListResp{
		Content: ret,
		Total:   int64(total),
	})
}

type ArchiveDecompressReq struct {
	SrcDir        string `json:"src_dir" form:"src_dir"`
	DstDir        string `json:"dst_dir" form:"dst_dir"`
	Name          string `json:"name" form:"name"`
	ArchivePass   string `json:"archive_pass" form:"archive_pass"`
	InnerPath     string `json:"inner_path" form:"inner_path"`
	CacheFull     bool   `json:"cache_full" form:"cache_full"`
	PutIntoNewDir bool   `json:"put_into_new_dir" form:"put_into_new_dir"`
}

func FsArchiveDecompress(c *gin.Context) {
	var req ArchiveDecompressReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if !user.CanDecompress() {
		common.ErrorResp(c, errs.PermissionDenied, 403)
		return
	}
	srcPath, err := user.JoinPath(stdpath.Join(req.SrcDir, req.Name))
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	dstDir, err := user.JoinPath(req.DstDir)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	t, err := fs.ArchiveDecompress(c, srcPath, dstDir, model.ArchiveDecompressArgs{
		ArchiveInnerArgs: model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					Header:  c.Request.Header,
					Type:    c.Query("type"),
					HttpReq: c.Request,
				},
				Password: req.ArchivePass,
			},
			InnerPath: utils.FixAndCleanPath(req.InnerPath),
		},
		CacheFull:     req.CacheFull,
		PutIntoNewDir: req.PutIntoNewDir,
	})
	if err != nil {
		if errors.Is(err, errs.WrongArchivePassword) {
			common.ErrorResp(c, err, 202)
		} else {
			common.ErrorResp(c, err, 500)
		}
		return
	}
	common.SuccessResp(c, gin.H{
		"task": getTaskInfo(t),
	})
}

func ArchiveDown(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	filename := stdpath.Base(innerPath)
	storage, err := fs.GetStorage(archiveRawPath, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if common.ShouldProxy(storage, filename) {
		ArchiveProxy(c)
		return
	} else {
		link, _, err := fs.ArchiveDriverExtract(c, archiveRawPath, model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					IP:       c.ClientIP(),
					Header:   c.Request.Header,
					Type:     c.Query("type"),
					HttpReq:  c.Request,
					Redirect: true,
				},
				Password: password,
			},
			InnerPath: innerPath,
		})
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		down(c, link)
	}
}

func ArchiveProxy(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	filename := stdpath.Base(innerPath)
	storage, err := fs.GetStorage(archiveRawPath, &fs.GetStoragesArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	if canProxy(storage, filename) {
		// TODO: Support external download proxy URL
		link, file, err := fs.ArchiveDriverExtract(c, archiveRawPath, model.ArchiveInnerArgs{
			ArchiveArgs: model.ArchiveArgs{
				LinkArgs: model.LinkArgs{
					Header:  c.Request.Header,
					Type:    c.Query("type"),
					HttpReq: c.Request,
				},
				Password: password,
			},
			InnerPath: innerPath,
		})
		if err != nil {
			common.ErrorResp(c, err, 500)
			return
		}
		localProxy(c, link, file, storage.GetStorage().ProxyRange)
	} else {
		common.ErrorStrResp(c, "proxy not allowed", 403)
		return
	}
}

func ArchiveInternalExtract(c *gin.Context) {
	archiveRawPath := c.MustGet("path").(string)
	innerPath := utils.FixAndCleanPath(c.Query("inner"))
	password := c.Query("pass")
	rc, size, err := fs.ArchiveInternalExtract(c, archiveRawPath, model.ArchiveInnerArgs{
		ArchiveArgs: model.ArchiveArgs{
			LinkArgs: model.LinkArgs{
				Header:  c.Request.Header,
				Type:    c.Query("type"),
				HttpReq: c.Request,
			},
			Password: password,
		},
		InnerPath: innerPath,
	})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Errorf("failed to close file streamer, %v", err)
		}
	}()
	headers := map[string]string{
		"Referrer-Policy": "no-referrer",
		"Cache-Control":   "max-age=0, no-cache, no-store, must-revalidate",
	}
	filename := stdpath.Base(innerPath)
	headers["Content-Disposition"] = fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, url.PathEscape(filename))
	contentType := c.Request.Header.Get("Content-Type")
	if contentType == "" {
		contentType = utils.GetMimeType(filename)
	}
	c.DataFromReader(200, size, contentType, rc, headers)
}

func ArchiveExtensions(c *gin.Context) {
	var ext []string
	for key := range tool.Tools {
		ext = append(ext, strings.TrimPrefix(key, "."))
	}
	common.SuccessResp(c, ext)
}
