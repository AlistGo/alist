package chunker

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func (d *Chunker) Config() driver.Config {
	return config
}

func (d *Chunker) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Chunker) Init(ctx context.Context) error {
	if d.ChunkSize == 0 {
		d.ChunkSize = defaultChunkSize
	}
	if d.StartFrom == 0 {
		d.StartFrom = defaultStartFrom
	}
	d.NameFormat = utils.GetNoneEmpty(d.NameFormat, defaultChunkNameFmt)
	d.MetaFormat = utils.GetNoneEmpty(d.MetaFormat, defaultMetaFormat)
	d.HashType = utils.GetNoneEmpty(d.HashType, defaultHashType)

	if err := d.setChunkNameFormat(d.NameFormat); err != nil {
		return fmt.Errorf("invalid name_format: %w", err)
	}
	if err := d.validateOptions(); err != nil {
		return err
	}

	targetPaths := d.configuredRemotePaths()
	d.remoteTargets = make([]remoteTarget, 0, len(targetPaths))
	for _, targetPath := range targetPaths {
		storage, err := fs.GetStorage(targetPath, &fs.GetStoragesArgs{})
		if err != nil {
			return fmt.Errorf("can't find remote storage %q: %w", targetPath, err)
		}
		d.remoteTargets = append(d.remoteTargets, remoteTarget{
			MountPath: targetPath,
			Storage:   storage,
		})
	}
	if len(d.remoteTargets) == 0 {
		return fmt.Errorf("can't find remote storage: %w", errs.ObjectNotFound)
	}
	d.remoteStorage = d.remoteTargets[0].Storage
	return nil
}

func (d *Chunker) Drop(ctx context.Context) error {
	d.remoteStorage = nil
	d.remoteTargets = nil
	return nil
}

func (d *Chunker) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return d.listDirObjects(ctx, dir.GetPath(), args.Refresh)
}

func (d *Chunker) Get(ctx context.Context, pathStr string) (model.Obj, error) {
	if utils.PathEqual(pathStr, "/") {
		return &model.Object{
			Name:     "Root",
			Path:     "/",
			IsFolder: true,
		}, nil
	}
	parent, name := path.Split(utils.FixAndCleanPath(pathStr))
	if parent == "" {
		parent = "/"
	}
	objs, err := d.listDirObjects(ctx, parent, false)
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		if obj.GetName() == name {
			return obj, nil
		}
	}
	return nil, errs.ObjectNotFound
}

func (d *Chunker) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	obj := d.linkedObject(file)
	if obj == nil || !obj.Chunked {
		remoteIndex := 0
		if obj != nil {
			remoteIndex = obj.MainRemoteIndex
		}
		actualPath, err := d.getActualPathForRemoteOnTarget(file.GetPath(), remoteIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		link, _, err := op.Link(ctx, d.remoteTargets[remoteIndex].Storage, actualPath, args)
		return link, err
	}

	linkedParts := make([]linkedPart, 0, len(obj.Parts))
	baseClosers := utils.EmptyClosers()
	for _, part := range obj.Parts {
		actualPath, err := d.getActualChunkPath(obj.GetPath(), part.No, part.XactID, part.RemoteIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to convert chunk path: %w", err)
		}
		link, _, err := op.Link(ctx, d.remoteTargets[part.RemoteIndex].Storage, actualPath, args)
		if err != nil {
			return nil, err
		}
		if link.MFile != nil {
			baseClosers.Add(link.MFile)
		}
		if link.RangeReadCloser != nil {
			baseClosers.Add(link.RangeReadCloser)
		}
		linkedParts = append(linkedParts, linkedPart{
			part: part,
			link: link,
		})
	}

	return &model.Link{
		RangeReadCloser: &model.RangeReadCloser{
			RangeReader: func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
				return d.openChunkReader(ctx, linkedParts, obj.GetSize(), httpRange)
			},
			Closers: baseClosers,
		},
	}, nil
}

func (d *Chunker) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	return d.ensureDirOnAllTargets(ctx, path.Join(parentDir.GetPath(), dirName))
}

func (d *Chunker) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcObj.IsDir() {
		return d.moveDirAcrossTargets(ctx, srcObj.GetPath(), dstDir.GetPath())
	}
	obj := d.linkedObject(srcObj)
	if obj == nil || !obj.Chunked {
		remoteIndex := 0
		if obj != nil {
			remoteIndex = obj.MainRemoteIndex
		}
		if err := d.ensureDirOnTarget(ctx, remoteIndex, dstDir.GetPath()); err != nil {
			return err
		}
		srcRemoteActualPath, err := d.getActualPathForRemoteOnTarget(srcObj.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		dstRemoteActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		return op.Move(ctx, d.remoteTargets[remoteIndex].Storage, srcRemoteActualPath, dstRemoteActualPath)
	}

	ensuredTargets := map[int]struct{}{}
	for _, location := range d.objectLocationsForObject(obj) {
		if _, ok := ensuredTargets[location.RemoteIndex]; !ok {
			if err := d.ensureDirOnTarget(ctx, location.RemoteIndex, dstDir.GetPath()); err != nil {
				return err
			}
			ensuredTargets[location.RemoteIndex] = struct{}{}
		}
		actualPath, err := d.getActualPathForRemoteOnTarget(location.LogicalPath, location.RemoteIndex)
		if err != nil {
			return err
		}
		dstRemoteActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), location.RemoteIndex)
		if err != nil {
			return err
		}
		if err := op.Move(ctx, d.remoteTargets[location.RemoteIndex].Storage, actualPath, dstRemoteActualPath); err != nil {
			return err
		}
	}
	return nil
}

func (d *Chunker) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	if srcObj.IsDir() {
		return d.renameDirAcrossTargets(ctx, srcObj.GetPath(), newName)
	}
	obj := d.linkedObject(srcObj)
	if obj == nil || !obj.Chunked {
		remoteIndex := 0
		if obj != nil {
			remoteIndex = obj.MainRemoteIndex
		}
		remoteActualPath, err := d.getActualPathForRemoteOnTarget(srcObj.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		return op.Rename(ctx, d.remoteTargets[remoteIndex].Storage, remoteActualPath, newName)
	}

	for _, part := range obj.Parts {
		actualPath, err := d.getActualChunkPath(obj.GetPath(), part.No, part.XactID, part.RemoteIndex)
		if err != nil {
			return err
		}
		newChunkName := d.chunkPartBaseName(path.Join(path.Dir(obj.GetPath()), newName), part.No, part.XactID)
		if err := op.Rename(ctx, d.remoteTargets[part.RemoteIndex].Storage, actualPath, newChunkName); err != nil {
			return err
		}
	}
	if obj.UsesMeta {
		actualPath, err := d.getActualPathForRemoteOnTarget(obj.GetPath(), obj.MainRemoteIndex)
		if err != nil {
			return err
		}
		if err := op.Rename(ctx, d.remoteTargets[obj.MainRemoteIndex].Storage, actualPath, newName); err != nil {
			return err
		}
	}
	return nil
}

func (d *Chunker) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if srcObj.IsDir() {
		return d.copyDirAcrossTargets(ctx, srcObj.GetPath(), dstDir.GetPath())
	}
	obj := d.linkedObject(srcObj)
	if obj == nil || !obj.Chunked {
		remoteIndex := 0
		if obj != nil {
			remoteIndex = obj.MainRemoteIndex
		}
		if err := d.ensureDirOnTarget(ctx, remoteIndex, dstDir.GetPath()); err != nil {
			return err
		}
		srcRemoteActualPath, err := d.getActualPathForRemoteOnTarget(srcObj.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		dstRemoteActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		return op.Copy(ctx, d.remoteTargets[remoteIndex].Storage, srcRemoteActualPath, dstRemoteActualPath)
	}

	ensuredTargets := map[int]struct{}{}
	for _, location := range d.objectLocationsForObject(obj) {
		if _, ok := ensuredTargets[location.RemoteIndex]; !ok {
			if err := d.ensureDirOnTarget(ctx, location.RemoteIndex, dstDir.GetPath()); err != nil {
				return err
			}
			ensuredTargets[location.RemoteIndex] = struct{}{}
		}
		actualPath, err := d.getActualPathForRemoteOnTarget(location.LogicalPath, location.RemoteIndex)
		if err != nil {
			return err
		}
		dstRemoteActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), location.RemoteIndex)
		if err != nil {
			return err
		}
		if err := op.Copy(ctx, d.remoteTargets[location.RemoteIndex].Storage, actualPath, dstRemoteActualPath); err != nil {
			return err
		}
	}
	return nil
}

func (d *Chunker) Remove(ctx context.Context, obj model.Obj) error {
	if obj.IsDir() {
		return d.removeDirAcrossTargets(ctx, obj.GetPath())
	}
	chunkedObj := d.linkedObject(obj)
	if chunkedObj == nil || !chunkedObj.Chunked {
		remoteIndex := 0
		if chunkedObj != nil {
			remoteIndex = chunkedObj.MainRemoteIndex
		}
		remoteActualPath, err := d.getActualPathForRemoteOnTarget(obj.GetPath(), remoteIndex)
		if err != nil {
			return fmt.Errorf("failed to convert path to remote path: %w", err)
		}
		return op.Remove(ctx, d.remoteTargets[remoteIndex].Storage, remoteActualPath)
	}

	for _, location := range d.objectLocationsForObject(chunkedObj) {
		actualPath, err := d.getActualPathForRemoteOnTarget(location.LogicalPath, location.RemoteIndex)
		if err != nil {
			return err
		}
		if err := op.Remove(ctx, d.remoteTargets[location.RemoteIndex].Storage, actualPath); err != nil {
			return err
		}
	}
	return nil
}

func (d *Chunker) Put(ctx context.Context, dstDir model.Obj, streamer model.FileStreamer, up driver.UpdateProgress) error {
	primaryDirActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), 0)
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	if err := d.ensureDirOnTarget(ctx, 0, dstDir.GetPath()); err != nil {
		return err
	}

	existing := d.linkedObject(streamer.GetExist())
	logicalPath := path.Join(dstDir.GetPath(), streamer.GetName())
	if streamer.GetSize() <= d.ChunkSize {
		if err := op.Put(ctx, d.remoteTargets[0].Storage, primaryDirActualPath, streamer, up, false); err != nil {
			return err
		}
		return d.cleanupReplacedObject(ctx, existing, d.buildKeepSet(d.targetLocation(logicalPath, 0)))
	}

	if up == nil {
		up = func(float64) {}
	}

	var (
		md5Hasher  hash.Hash
		sha1Hasher hash.Hash
		writers    []io.Writer
	)
	switch d.HashType {
	case "md5":
		md5Hasher = md5.New()
		writers = append(writers, md5Hasher)
	case "sha1":
		sha1Hasher = sha1.New()
		writers = append(writers, sha1Hasher)
	}
	writers = append(writers, driver.NewProgress(streamer.GetSize(), up))

	baseReader := io.TeeReader(streamer, io.MultiWriter(writers...))
	xactID := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(xactID) > 9 {
		xactID = xactID[len(xactID)-9:]
	}
	if len(xactID) < 4 {
		xactID = fmt.Sprintf("%04s", xactID)
	}

	chunkCount := 0
	remaining := streamer.GetSize()
	keepLocations := make([]objectLocation, 0, len(d.remoteTargets)+1)
	ensuredTargets := map[int]struct{}{0: {}}
	for remaining > 0 {
		chunkLen := utils.Min(remaining, d.ChunkSize)
		targetIndex := d.chunkTargetIndex(chunkCount)
		if _, ok := ensuredTargets[targetIndex]; !ok {
			if err := d.ensureDirOnTarget(ctx, targetIndex, dstDir.GetPath()); err != nil {
				return err
			}
			ensuredTargets[targetIndex] = struct{}{}
		}
		dstDirActualPath, err := d.getActualPathForRemoteOnTarget(dstDir.GetPath(), targetIndex)
		if err != nil {
			return err
		}
		chunkName := d.chunkPartBaseName(logicalPath, chunkCount, xactIDIfNeeded(d.MetaFormat, xactID))
		chunkPath := d.makeChunkName(logicalPath, chunkCount, xactIDIfNeeded(d.MetaFormat, xactID))
		partReader := driver.NewLimitedUploadStream(ctx, &driver.ReaderWithCtx{
			Reader: io.LimitReader(baseReader, chunkLen),
			Ctx:    ctx,
		})
		partStream := &stream.FileStream{
			Obj: &model.Object{
				Name:     chunkName,
				Size:     chunkLen,
				Modified: streamer.ModTime(),
				Ctime:    streamer.CreateTime(),
				IsFolder: false,
			},
			Reader:            partReader,
			Mimetype:          "application/octet-stream",
			WebPutAsTask:      streamer.NeedStore(),
			ForceStreamUpload: true,
		}
		if err := op.Put(ctx, d.remoteTargets[targetIndex].Storage, dstDirActualPath, partStream, nil, false); err != nil {
			return err
		}
		keepLocations = append(keepLocations, d.targetLocation(chunkPath, targetIndex))
		remaining -= chunkLen
		chunkCount++
	}

	if d.MetaFormat == "simplejson" {
		md5Value := ""
		if md5Hasher != nil {
			md5Value = hex.EncodeToString(md5Hasher.Sum(nil))
		}
		sha1Value := ""
		if sha1Hasher != nil {
			sha1Value = hex.EncodeToString(sha1Hasher.Sum(nil))
		}
		txn := xactID
		metaData, err := marshalMetadata(streamer.GetSize(), chunkCount, md5Value, sha1Value, txn)
		if err != nil {
			return err
		}
		metaStream := &stream.FileStream{
			Obj: &model.Object{
				Name:     streamer.GetName(),
				Size:     int64(len(metaData)),
				Modified: streamer.ModTime(),
				Ctime:    streamer.CreateTime(),
				IsFolder: false,
			},
			Reader:            bytes.NewReader(metaData),
			Mimetype:          "application/json",
			WebPutAsTask:      false,
			ForceStreamUpload: true,
		}
		if err := op.Put(ctx, d.remoteTargets[0].Storage, primaryDirActualPath, metaStream, nil, false); err != nil {
			return err
		}
		keepLocations = append(keepLocations, d.targetLocation(logicalPath, 0))
	} else {
		for remoteIndex := range d.remoteTargets {
			actualPath, err := d.getActualPathForRemoteOnTarget(logicalPath, remoteIndex)
			if err == nil {
				_ = op.Remove(ctx, d.remoteTargets[remoteIndex].Storage, actualPath)
			}
		}
	}

	return d.cleanupReplacedObject(ctx, existing, d.buildKeepSet(keepLocations...))
}

func xactIDIfNeeded(metaFormat, xactID string) string {
	if metaFormat == "simplejson" {
		return xactID
	}
	return ""
}

var _ driver.Driver = (*Chunker)(nil)
