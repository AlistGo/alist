package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	stdpath "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	log "github.com/sirupsen/logrus"
)

const (
	chunkUploadTempDirName          = "alist-chunk"
	chunkUploadTempFileSuffix       = ".part"
	chunkUploadSessionTTL           = 45 * time.Minute
	chunkUploadReaperInterval       = 10 * time.Minute
	maxUploadChunkSizeMB      int64 = 4096
)

const (
	chunkUploadStateActive     = "active"
	chunkUploadStateCompleting = "completing"
	chunkUploadStateFailed     = "failed"
	chunkUploadStateCanceled   = "canceled"
	chunkUploadStateCompleted  = "completed"
)

var (
	ErrChunkUploadNotFound   = errors.New("chunk upload session not found")
	ErrChunkUploadNotOwner   = errors.New("chunk upload session not owned by current user")
	ErrChunkUploadNeedRetry  = errors.New("chunk upload failed, please retry from start")
	ErrChunkUploadDisabled   = errors.New("chunk upload disabled for current storage")
	ErrChunkUploadBadRequest = errors.New("invalid chunk upload request")
	ErrChunkUploadFileExists = errors.New("file exists")

	chunkUploadSessions   sync.Map // map[string]*chunkUploadSession
	chunkUploadReaperOnce sync.Once
)

type UploadPolicy struct {
	Provider    string
	ChunkSizeMB int64
}

type ChunkUploadInitArgs struct {
	UserID       uint
	Path         string
	Size         int64
	AsTask       bool
	Overwrite    bool
	LastModified time.Time
	Mimetype     string
	Hashes       map[*utils.HashType]string
}

type ChunkUploadInitResp struct {
	UploadID       string
	Provider       string
	ChunkSizeMB    int64
	ChunkSizeBytes int64
}

type chunkUploadSession struct {
	mu sync.Mutex

	UploadID       string
	UserID         uint
	Path           string
	DirPath        string
	FileName       string
	TempPath       string
	Size           int64
	ChunkSizeBytes int64
	UploadedSize   int64
	NextChunkIndex int64
	AsTask         bool
	Overwrite      bool
	Mimetype       string
	LastModified   time.Time
	Hashes         map[*utils.HashType]string
	UpdatedAt      time.Time
	State          string
}

func chunkUploadRootPath() string {
	return filepath.Join(conf.Conf.TempDir, chunkUploadTempDirName)
}

func chunkUploadTempPath(uploadID string) string {
	return filepath.Join(chunkUploadRootPath(), uploadID+chunkUploadTempFileSuffix)
}

func StartChunkUploadReaper() {
	chunkUploadReaperOnce.Do(func() {
		root := chunkUploadRootPath()
		if err := os.MkdirAll(root, 0o777); err != nil {
			log.Errorf("[chunk-upload] failed to create temp dir %s: %+v", root, err)
			return
		}
		cleanupStaleChunkUploadParts(true)
		go func() {
			ticker := time.NewTicker(chunkUploadReaperInterval)
			defer ticker.Stop()
			for range ticker.C {
				cleanupStaleChunkUploadSessions()
				cleanupStaleChunkUploadParts(false)
			}
		}()
	})
}

func cleanupStaleChunkUploadSessions() {
	now := time.Now()
	chunkUploadSessions.Range(func(key, value any) bool {
		uploadID, ok := key.(string)
		if !ok {
			return true
		}
		session, ok := value.(*chunkUploadSession)
		if !ok {
			return true
		}
		session.mu.Lock()
		expired := now.Sub(session.UpdatedAt) > chunkUploadSessionTTL
		if expired {
			session.State = chunkUploadStateFailed
		}
		session.mu.Unlock()
		if expired {
			if err := CleanupChunkUpload(uploadID); err != nil {
				log.Warnf("[chunk-upload] failed cleanup expired session %s: %+v", uploadID, err)
			}
		}
		return true
	})
}

func cleanupStaleChunkUploadParts(force bool) {
	root := chunkUploadRootPath()
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warnf("[chunk-upload] failed to scan temp dir %s: %+v", root, err)
		}
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, chunkUploadTempFileSuffix) {
			continue
		}
		fullPath := filepath.Join(root, name)
		if !force {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) <= chunkUploadSessionTTL {
				continue
			}
		}
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			log.Warnf("[chunk-upload] failed removing stale part %s: %+v", fullPath, err)
		}
	}
}

func GetUploadPolicy(path string) (UploadPolicy, error) {
	storage, err := GetStorage(path, &GetStoragesArgs{})
	if err != nil {
		return UploadPolicy{}, err
	}
	policy := UploadPolicy{Provider: storage.Config().Name}
	if policy.Provider != "Local" {
		return policy, nil
	}
	additionRaw := storage.GetStorage().Addition
	if additionRaw == "" {
		return policy, nil
	}
	addition := map[string]any{}
	if err := utils.Json.UnmarshalFromString(additionRaw, &addition); err != nil {
		return policy, nil
	}
	v, ok := addition["upload_chunk_size_mb"]
	if !ok {
		return policy, nil
	}
	chunkMB, ok := toInt64(v)
	if !ok {
		return policy, nil
	}
	if chunkMB < 0 {
		chunkMB = 0
	}
	if chunkMB > maxUploadChunkSizeMB {
		chunkMB = maxUploadChunkSizeMB
	}
	policy.ChunkSizeMB = chunkMB
	return policy, nil
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint:
		if uint64(n) > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(n), true
	case string:
		p, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err != nil {
			return 0, false
		}
		return p, true
	default:
		return 0, false
	}
}

func InitChunkUpload(ctx context.Context, args ChunkUploadInitArgs) (*ChunkUploadInitResp, error) {
	if args.Path == "" {
		return nil, fmt.Errorf("%w: path is required", ErrChunkUploadBadRequest)
	}
	if args.Size <= 0 {
		return nil, fmt.Errorf("%w: file size must be greater than 0", ErrChunkUploadBadRequest)
	}
	dirPath, fileName := stdpath.Split(args.Path)
	if fileName == "" {
		return nil, fmt.Errorf("%w: invalid file path", ErrChunkUploadBadRequest)
	}
	policy, err := GetUploadPolicy(args.Path)
	if err != nil {
		return nil, err
	}
	if policy.ChunkSizeMB <= 0 {
		return nil, ErrChunkUploadDisabled
	}
	if !args.Overwrite {
		if obj, _ := Get(ctx, args.Path, &GetArgs{NoLog: true}); obj != nil {
			return nil, ErrChunkUploadFileExists
		}
	}
	if err := os.MkdirAll(chunkUploadRootPath(), 0o777); err != nil {
		return nil, err
	}

	chunkSizeBytes := policy.ChunkSizeMB * 1024 * 1024
	var (
		uploadID string
		tempPath string
	)
	for i := 0; i < 4; i++ {
		uploadID = random.String(24)
		tempPath = chunkUploadTempPath(uploadID)
		if _, ok := chunkUploadSessions.Load(uploadID); ok {
			continue
		}
		part, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, err
		}
		_ = part.Close()
		session := &chunkUploadSession{
			UploadID:       uploadID,
			UserID:         args.UserID,
			Path:           args.Path,
			DirPath:        dirPath,
			FileName:       fileName,
			TempPath:       tempPath,
			Size:           args.Size,
			ChunkSizeBytes: chunkSizeBytes,
			AsTask:         args.AsTask,
			Overwrite:      args.Overwrite,
			Mimetype:       args.Mimetype,
			LastModified:   args.LastModified,
			Hashes:         cloneHashes(args.Hashes),
			UpdatedAt:      time.Now(),
			State:          chunkUploadStateActive,
		}
		chunkUploadSessions.Store(uploadID, session)
		return &ChunkUploadInitResp{
			UploadID:       uploadID,
			Provider:       policy.Provider,
			ChunkSizeMB:    policy.ChunkSizeMB,
			ChunkSizeBytes: chunkSizeBytes,
		}, nil
	}
	return nil, fmt.Errorf("%w: failed to allocate upload session", ErrChunkUploadNeedRetry)
}

func cloneHashes(src map[*utils.HashType]string) map[*utils.HashType]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[*utils.HashType]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func AppendChunk(ctx context.Context, uploadID string, userID uint, chunkIndex int64, chunk io.Reader) (uploaded int64, total int64, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	session, err := getChunkUploadSession(uploadID)
	if err != nil {
		return 0, 0, err
	}

	session.mu.Lock()
	if session.UserID != userID {
		session.mu.Unlock()
		return 0, 0, ErrChunkUploadNotOwner
	}
	if session.State != chunkUploadStateActive {
		session.mu.Unlock()
		return 0, 0, fmt.Errorf("%w: upload session is not active", ErrChunkUploadNeedRetry)
	}
	if chunkIndex != session.NextChunkIndex {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, fmt.Errorf("%w: expected chunk index %d, got %d", ErrChunkUploadNeedRetry, session.NextChunkIndex, chunkIndex)
	}
	remaining := session.Size - session.UploadedSize
	if remaining <= 0 {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, fmt.Errorf("%w: upload session already complete", ErrChunkUploadNeedRetry)
	}
	expected := session.ChunkSizeBytes
	if remaining < expected {
		expected = remaining
	}
	if err := ctx.Err(); err != nil {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, err
	}

	partFile, openErr := os.OpenFile(session.TempPath, os.O_WRONLY|os.O_APPEND, 0o666)
	if openErr != nil {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, openErr
	}
	written, copyErr := utils.CopyWithBuffer(partFile, chunk)
	closeErr := partFile.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, copyErr
	}
	if written != expected {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return 0, 0, fmt.Errorf("%w: chunk size mismatch, expected=%d actual=%d", ErrChunkUploadNeedRetry, expected, written)
	}

	session.UploadedSize += written
	session.NextChunkIndex++
	session.UpdatedAt = time.Now()
	uploaded = session.UploadedSize
	total = session.Size
	session.mu.Unlock()
	return uploaded, total, nil
}

func CompleteChunkUpload(ctx context.Context, uploadID string, userID uint) (task.TaskExtensionInfo, error) {
	session, err := getChunkUploadSession(uploadID)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	if session.UserID != userID {
		session.mu.Unlock()
		return nil, ErrChunkUploadNotOwner
	}
	if session.State != chunkUploadStateActive {
		session.mu.Unlock()
		return nil, fmt.Errorf("%w: upload session is not active", ErrChunkUploadNeedRetry)
	}
	if session.UploadedSize != session.Size {
		session.State = chunkUploadStateFailed
		session.UpdatedAt = time.Now()
		session.mu.Unlock()
		_ = CleanupChunkUpload(uploadID)
		return nil, fmt.Errorf("%w: uploaded size mismatch, expected=%d actual=%d", ErrChunkUploadNeedRetry, session.Size, session.UploadedSize)
	}
	session.State = chunkUploadStateCompleting
	session.UpdatedAt = time.Now()

	tempPath := session.TempPath
	dirPath := session.DirPath
	fileName := session.FileName
	fileSize := session.Size
	mimetype := session.Mimetype
	lastModified := session.LastModified
	asTask := session.AsTask
	hashes := cloneHashes(session.Hashes)
	session.mu.Unlock()

	handoffFile, err := os.CreateTemp(conf.Conf.TempDir, "chunk-upload-*")
	if err != nil {
		_ = failAndCleanupChunkUpload(uploadID)
		return nil, err
	}
	handoffPath := handoffFile.Name()
	_ = handoffFile.Close()

	if err := os.Rename(tempPath, handoffPath); err != nil {
		_ = os.Remove(handoffPath)
		_ = failAndCleanupChunkUpload(uploadID)
		return nil, err
	}

	session.mu.Lock()
	session.State = chunkUploadStateCompleted
	session.UpdatedAt = time.Now()
	session.mu.Unlock()

	chunkUploadSessions.Delete(uploadID)

	tmp, err := os.Open(handoffPath)
	if err != nil {
		_ = os.Remove(handoffPath)
		return nil, err
	}
	if lastModified.IsZero() {
		lastModified = time.Now()
	}
	fileStream := &stream.FileStream{
		Obj: &model.Object{
			Name:     fileName,
			Size:     fileSize,
			Modified: lastModified,
			HashInfo: utils.NewHashInfoByMap(hashes),
		},
		Mimetype:     mimetype,
		WebPutAsTask: asTask,
	}
	fileStream.SetTmpFile(tmp)

	if asTask {
		t, err := putAsTask(ctx, dirPath, fileStream)
		if err != nil {
			_ = fileStream.Close()
			return nil, err
		}
		return t, nil
	}
	err = putDirectly(ctx, dirPath, fileStream, true)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func CancelChunkUpload(uploadID string, userID uint) error {
	session, err := getChunkUploadSession(uploadID)
	if err != nil {
		return err
	}
	session.mu.Lock()
	if session.UserID != userID {
		session.mu.Unlock()
		return ErrChunkUploadNotOwner
	}
	session.State = chunkUploadStateCanceled
	session.UpdatedAt = time.Now()
	session.mu.Unlock()
	return CleanupChunkUpload(uploadID)
}

func failAndCleanupChunkUpload(uploadID string) error {
	return CleanupChunkUpload(uploadID)
}

func CleanupChunkUpload(uploadID string) error {
	tempPath := chunkUploadTempPath(uploadID)
	if value, ok := chunkUploadSessions.LoadAndDelete(uploadID); ok {
		if session, ok := value.(*chunkUploadSession); ok {
			session.mu.Lock()
			if session.TempPath != "" {
				tempPath = session.TempPath
			}
			session.mu.Unlock()
		}
	}
	err := os.Remove(tempPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func getChunkUploadSession(uploadID string) (*chunkUploadSession, error) {
	v, ok := chunkUploadSessions.Load(uploadID)
	if !ok {
		return nil, ErrChunkUploadNotFound
	}
	session, ok := v.(*chunkUploadSession)
	if !ok {
		chunkUploadSessions.Delete(uploadID)
		return nil, ErrChunkUploadNotFound
	}
	return session, nil
}
