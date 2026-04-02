package handles

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	stdpath "path"
	"regexp"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/db"
	shareauth "github.com/alist-org/alist/v3/internal/share"

	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
)

const shareAccessTokenLifetime = 24 * time.Hour

var shareIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

var (
	errShareIDInvalid = errors.New("share_id must be 1-32 characters of letters, numbers, underscore or hyphen")
	errShareIDExists  = errors.New("share link already exists")
)

type CreateShareReq struct {
	Path          string `json:"path" binding:"required"`
	ShareID       string `json:"share_id"`
	Name          string `json:"name"`
	Password      string `json:"password"`
	ExpireAt      string `json:"expire_at"`
	ExpireHours   int64  `json:"expire_hours"`
	AccessLimit   int64  `json:"access_limit"`
	BurnAfterRead *bool  `json:"burn_after_read"`
	AllowPreview  *bool  `json:"allow_preview"`
	AllowDownload *bool  `json:"allow_download"`
}

type UpdateShareReq struct {
	ShareID       string  `json:"share_id" binding:"required"`
	NewShareID    string  `json:"new_share_id"`
	Name          string  `json:"name"`
	Password      string  `json:"password"`
	ExpireAt      *string `json:"expire_at"`
	AccessLimit   *int64  `json:"access_limit"`
	AllowPreview  *bool   `json:"allow_preview"`
	AllowDownload *bool   `json:"allow_download"`
}

type ShareDeleteReq struct {
	ShareID string `json:"share_id" binding:"required"`
}

type ShareAuthReq struct {
	ShareID  string `json:"share_id" binding:"required"`
	Password string `json:"password"`
}

type PublicShareReq struct {
	ShareID string `json:"share_id" binding:"required"`
	Path    string `json:"path"`
	Token   string `json:"token"`
}

type PublicShareListReq struct {
	model.PageReq
	ShareID string `json:"share_id" binding:"required"`
	Path    string `json:"path"`
	Token   string `json:"token"`
}

type ShareResp struct {
	ID                uint       `json:"id"`
	ShareID           string     `json:"share_id"`
	Name              string     `json:"name"`
	RootPath          string     `json:"root_path"`
	IsDir             bool       `json:"is_dir"`
	HasPassword       bool       `json:"has_password"`
	BurnAfterRead     bool       `json:"burn_after_read"`
	AccessLimit       int64      `json:"access_limit"`
	AccessCount       int64      `json:"access_count"`
	RemainingAccesses int64      `json:"remaining_accesses"`
	AllowPreview      bool       `json:"allow_preview"`
	AllowDownload     bool       `json:"allow_download"`
	Enabled           bool       `json:"enabled"`
	ViewCount         int64      `json:"view_count"`
	DownloadCount     int64      `json:"download_count"`
	LastAccessAt      *time.Time `json:"last_access_at"`
	ConsumedAt        *time.Time `json:"consumed_at"`
	ExpiresAt         *time.Time `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	URL               string     `json:"url"`
}

type PublicShareInfoResp struct {
	ShareID           string     `json:"share_id"`
	Name              string     `json:"name"`
	IsDir             bool       `json:"is_dir"`
	HasPassword       bool       `json:"has_password"`
	BurnAfterRead     bool       `json:"burn_after_read"`
	AccessLimit       int64      `json:"access_limit"`
	AccessCount       int64      `json:"access_count"`
	RemainingAccesses int64      `json:"remaining_accesses"`
	AllowPreview      bool       `json:"allow_preview"`
	AllowDownload     bool       `json:"allow_download"`
	Authed            bool       `json:"authed"`
	ConsumedAt        *time.Time `json:"consumed_at"`
	ExpiresAt         *time.Time `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

type PublicShareObjResp struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	IsDir        bool      `json:"is_dir"`
	Modified     time.Time `json:"modified"`
	Created      time.Time `json:"created"`
	Thumb        string    `json:"thumb"`
	Type         int       `json:"type"`
	Path         string    `json:"path"`
	StorageClass string    `json:"storage_class,omitempty"`
	DownloadURL  string    `json:"download_url,omitempty"`
	PreviewURL   string    `json:"preview_url,omitempty"`
}

type PublicShareListResp struct {
	Content    []PublicShareObjResp `json:"content"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	HasMore    bool                 `json:"has_more"`
	PagesTotal int                  `json:"pages_total"`
}

type PublicShareGetResp struct {
	Item     PublicShareObjResp `json:"item"`
	Provider string             `json:"provider"`
}

func shareURL(c *gin.Context, shareID string) string {
	return fmt.Sprintf("%s/s/%s", common.GetApiUrl(c.Request), shareID)
}

func toShareResp(c *gin.Context, share *model.Share) ShareResp {
	accessLimit := share.EffectiveAccessLimit()
	return ShareResp{
		ID:                share.ID,
		ShareID:           share.ShareID,
		Name:              share.Name,
		RootPath:          share.RootPath,
		IsDir:             share.IsDir,
		HasPassword:       share.HasPassword(),
		BurnAfterRead:     accessLimit == 1,
		AccessLimit:       accessLimit,
		AccessCount:       share.AccessCount,
		RemainingAccesses: share.RemainingAccesses(),
		AllowPreview:      share.AllowPreview,
		AllowDownload:     share.AllowDownload,
		Enabled:           share.Enabled,
		ViewCount:         share.ViewCount,
		DownloadCount:     share.DownloadCount,
		LastAccessAt:      share.LastAccessAt,
		ConsumedAt:        share.ConsumedAt,
		ExpiresAt:         share.ExpiresAt,
		CreatedAt:         share.CreatedAt,
		UpdatedAt:         share.UpdatedAt,
		URL:               shareURL(c, share.ShareID),
	}
}

func normalizeShareName(obj model.Obj, name string) string {
	return normalizeOptionalShareName(name, obj.GetName())
}

func normalizeOptionalShareName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return fallback
}

func generateShareID() (string, error) {
	for range 10 {
		shareID := random.String(8)
		exists, err := db.ShareIDExists(shareID)
		if err != nil {
			return "", err
		}
		if !exists {
			return shareID, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique share id")
}

func sharePasswordHash(password, salt string) string {
	return model.HashPwd(model.StaticHash(password), salt)
}

func validateCustomShareID(shareID string) error {
	if shareID == "" {
		return nil
	}
	if !shareIDPattern.MatchString(shareID) {
		return errShareIDInvalid
	}
	return nil
}

func resolveRequestedShareID(rawShareID, fallback string, excludeID uint) (string, error) {
	shareID := strings.TrimSpace(rawShareID)
	if shareID == "" {
		if fallback != "" {
			return fallback, nil
		}
		return generateShareID()
	}
	if err := validateCustomShareID(shareID); err != nil {
		return "", err
	}
	if excludeID == 0 {
		exists, err := db.ShareIDExists(shareID)
		if err != nil {
			return "", fmt.Errorf("check share id availability: %w", err)
		}
		if exists {
			return "", errShareIDExists
		}
		return shareID, nil
	}
	exists, err := db.ShareIDExistsExceptID(shareID, excludeID)
	if err != nil {
		return "", fmt.Errorf("check share id availability: %w", err)
	}
	if exists {
		return "", errShareIDExists
	}
	return shareID, nil
}

func normalizeShareAccessLimit(accessLimit int64, burnAfterRead *bool) (int64, bool, error) {
	if accessLimit < 0 {
		return 0, false, fmt.Errorf("access_limit must be 0 or greater")
	}
	if accessLimit == 0 && burnAfterRead != nil && *burnAfterRead {
		accessLimit = 1
	}
	return accessLimit, accessLimit == 1, nil
}

func parseShareExpireAt(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, nil
	}
	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("invalid expire_at")
}

func resolveShareExpireAt(expireAt string, expireHours int64) (*time.Time, error) {
	if strings.TrimSpace(expireAt) != "" {
		return parseShareExpireAt(expireAt)
	}
	if expireHours < 0 {
		return nil, fmt.Errorf("expire_hours must be 0 or greater")
	}
	if expireHours == 0 {
		return nil, nil
	}
	expires := time.Now().Add(time.Duration(expireHours) * time.Hour)
	return &expires, nil
}

func sharePasswordMatched(share *model.Share, password string) bool {
	if !share.HasPassword() {
		return true
	}
	hash := sharePasswordHash(password, share.PasswordSalt)
	return subtle.ConstantTimeCompare([]byte(hash), []byte(share.PasswordHash)) == 1
}

func getShareAccessToken(c *gin.Context, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if token := c.Query("auth"); token != "" {
		return token
	}
	return c.GetHeader("X-Share-Token")
}

func ensureShareAvailable(c *gin.Context, share *model.Share) bool {
	now := time.Now()
	if share.IsConsumed() {
		common.ErrorStrResp(c, "share has been consumed", 410)
		return false
	}
	if !share.Enabled {
		common.ErrorStrResp(c, "share is disabled", 404)
		return false
	}
	if share.IsExpired(now) {
		common.ErrorStrResp(c, "share is expired", 410)
		return false
	}
	return true
}

func recordShareAccess(share *model.Share) error {
	updated, err := db.RecordShareAccess(share.ShareID)
	if err != nil {
		return err
	}
	if updated != nil {
		*share = *updated
	}
	return nil
}

func ensureShareAccess(c *gin.Context, share *model.Share, token string) bool {
	if !share.HasPassword() {
		return true
	}
	if token == "" {
		common.ErrorStrResp(c, "share password required", 401)
		return false
	}
	if err := shareauth.VerifyAccess(share, token); err != nil {
		common.ErrorResp(c, err, 401)
		return false
	}
	return true
}

func shouldTrackShareContentAccess(c *gin.Context) bool {
	return c.Request.Method != http.MethodHead
}

func resolveShareTarget(share *model.Share, rawRelPath string) (string, string, error) {
	cleanRelPath := utils.FixAndCleanPath(rawRelPath)
	if !share.IsDir && cleanRelPath != "/" {
		return "", "", fmt.Errorf("file share does not support nested path")
	}
	if cleanRelPath == "/" {
		return share.RootPath, "/", nil
	}
	target := utils.FixAndCleanPath(stdpath.Join(share.RootPath, cleanRelPath))
	if !utils.IsSubPath(share.RootPath, target) {
		return "", "", fmt.Errorf("share path out of range")
	}
	return target, cleanRelPath, nil
}

func resolveShareWildcardTarget(share *model.Share, rawPath string) (string, string, error) {
	path, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", "", err
	}
	return resolveShareTarget(share, strings.TrimPrefix(path, "/"))
}

func buildPublicShareAssetURL(c *gin.Context, prefix, shareID, relPath, token string, preview bool) string {
	base := common.GetApiUrl(c.Request) + prefix + shareID
	cleanPath := utils.FixAndCleanPath(relPath)
	if cleanPath != "/" {
		base += utils.EncodePath(cleanPath, true)
	}
	query := url.Values{}
	if token != "" {
		query.Set("auth", token)
	}
	if preview {
		query.Set("type", "preview")
	}
	if encoded := query.Encode(); encoded != "" {
		base += "?" + encoded
	}
	return base
}

func buildPublicSharePreviewURL(c *gin.Context, obj model.Obj, targetPath, shareID, relPath, token string) string {
	prefix := "/sd/"
	storage, err := fs.GetStorage(targetPath, &fs.GetStoragesArgs{})
	if err == nil && canProxy(storage, obj.GetName()) {
		prefix = "/sp/"
	}
	return buildPublicShareAssetURL(c, prefix, shareID, relPath, token, true)
}

func toPublicShareObjResp(c *gin.Context, share *model.Share, obj model.Obj, targetPath, relPath, token string) PublicShareObjResp {
	thumb, _ := model.GetThumb(obj)
	storageClass, _ := model.GetStorageClass(obj)
	resp := PublicShareObjResp{
		Name:         obj.GetName(),
		Size:         obj.GetSize(),
		IsDir:        obj.IsDir(),
		Modified:     obj.ModTime(),
		Created:      obj.CreateTime(),
		Thumb:        thumb,
		Type:         utils.GetObjType(obj.GetName(), obj.IsDir()),
		Path:         relPath,
		StorageClass: storageClass,
	}
	if !obj.IsDir() && share.AllowDownload {
		resp.DownloadURL = buildPublicShareAssetURL(c, "/sd/", share.ShareID, relPath, token, false)
	}
	if !obj.IsDir() && share.AllowPreview {
		resp.PreviewURL = buildPublicSharePreviewURL(c, obj, targetPath, share.ShareID, relPath, token)
	}
	return resp
}

func CreateShare(c *gin.Context) {
	var req CreateShareReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	reqPath, err := user.JoinPath(req.Path)
	if err != nil {
		common.ErrorResp(c, err, 403)
		return
	}
	if !common.CanReadPathByRole(user, reqPath) {
		common.ErrorStrResp(c, "you have no permission", 403)
		return
	}
	obj, err := fs.Get(c, reqPath, &fs.GetArgs{})
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	shareID, err := resolveRequestedShareID(req.ShareID, "", 0)
	if err != nil {
		if errors.Is(err, errShareIDInvalid) || errors.Is(err, errShareIDExists) {
			common.ErrorResp(c, err, 400)
			return
		}
		common.ErrorResp(c, err, 500, true)
		return
	}
	allowPreview := true
	if req.AllowPreview != nil {
		allowPreview = *req.AllowPreview
	}
	allowDownload := true
	if req.AllowDownload != nil {
		allowDownload = *req.AllowDownload
	}
	accessLimit, burnAfterRead, err := normalizeShareAccessLimit(req.AccessLimit, req.BurnAfterRead)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	expiresAt, err := resolveShareExpireAt(req.ExpireAt, req.ExpireHours)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	share := &model.Share{
		ShareID:       shareID,
		CreatorID:     user.ID,
		Name:          normalizeShareName(obj, req.Name),
		RootPath:      reqPath,
		IsDir:         obj.IsDir(),
		BurnAfterRead: burnAfterRead,
		AccessLimit:   accessLimit,
		AllowPreview:  allowPreview,
		AllowDownload: allowDownload,
		Enabled:       true,
		ExpiresAt:     expiresAt,
	}
	if req.Password != "" {
		share.PasswordSalt = random.String(16)
		share.PasswordHash = sharePasswordHash(req.Password, share.PasswordSalt)
	}
	if err := db.CreateShare(share); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, toShareResp(c, share))
}

func UpdateShare(c *gin.Context) {
	var req UpdateShareReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	share, err := db.GetShareByCreatorAndShareID(user.ID, req.ShareID)
	if err != nil {
		common.ErrorResp(c, err, 404)
		return
	}

	shareID, err := resolveRequestedShareID(req.NewShareID, share.ShareID, share.ID)
	if err != nil {
		if errors.Is(err, errShareIDInvalid) || errors.Is(err, errShareIDExists) {
			common.ErrorResp(c, err, 400)
			return
		}
		common.ErrorResp(c, err, 500, true)
		return
	}

	allowPreview := share.AllowPreview
	if req.AllowPreview != nil {
		allowPreview = *req.AllowPreview
	}
	allowDownload := share.AllowDownload
	if req.AllowDownload != nil {
		allowDownload = *req.AllowDownload
	}

	accessLimit := share.EffectiveAccessLimit()
	burnAfterRead := accessLimit == 1
	if req.AccessLimit != nil {
		accessLimit, burnAfterRead, err = normalizeShareAccessLimit(*req.AccessLimit, nil)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
	}

	expiresAt := share.ExpiresAt
	if req.ExpireAt != nil {
		expiresAt, err = parseShareExpireAt(*req.ExpireAt)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
	}

	share.ShareID = shareID
	share.Name = normalizeOptionalShareName(req.Name, share.Name)
	share.BurnAfterRead = burnAfterRead
	share.AccessLimit = accessLimit
	share.AllowPreview = allowPreview
	share.AllowDownload = allowDownload
	share.ExpiresAt = expiresAt
	if req.Password != "" {
		share.PasswordSalt = random.String(16)
		share.PasswordHash = sharePasswordHash(req.Password, share.PasswordSalt)
	}
	if share.Enabled && accessLimit > 0 && share.AccessCount >= accessLimit {
		now := time.Now()
		share.Enabled = false
		if share.ConsumedAt == nil {
			share.ConsumedAt = &now
		}
	}
	if err := db.UpdateShare(share); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, toShareResp(c, share))
}

func ListShares(c *gin.Context) {
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	user := c.MustGet("user").(*model.User)
	shares, total, err := db.GetSharesByCreator(user.ID, req.Page, req.PerPage)
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	resp := make([]ShareResp, 0, len(shares))
	for i := range shares {
		resp = append(resp, toShareResp(c, &shares[i]))
	}
	common.SuccessResp(c, common.PageResp{
		Content: resp,
		Total:   total,
	})
}

func DisableShare(c *gin.Context) {
	var req ShareDeleteReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if _, err := db.GetShareByCreatorAndShareID(user.ID, req.ShareID); err != nil {
		common.ErrorResp(c, err, 404)
		return
	}
	if err := db.DisableShareByShareID(user.ID, req.ShareID); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func DeleteShare(c *gin.Context) {
	var req ShareDeleteReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	user := c.MustGet("user").(*model.User)
	if err := db.DeleteShareByShareID(user.ID, req.ShareID); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}
