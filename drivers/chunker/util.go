package chunker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
)

func (d *Chunker) validateOptions() error {
	if strings.TrimSpace(d.RemotePath) == "" {
		return errors.New("remote_path is required")
	}
	if d.ChunkSize <= 0 {
		return errors.New("chunk_size must be positive")
	}
	switch d.MetaFormat {
	case "simplejson", "none":
	default:
		return fmt.Errorf("unsupported meta_format: %s", d.MetaFormat)
	}
	switch d.HashType {
	case "none", "md5", "sha1":
	default:
		return fmt.Errorf("unsupported hash_type: %s", d.HashType)
	}
	if d.MetaFormat == "none" && d.HashType != "none" {
		return fmt.Errorf("hash_type %q requires meta_format=simplejson", d.HashType)
	}
	return nil
}

func (d *Chunker) configuredRemotePaths() []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, 1)
	addPath := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		p = utils.FixAndCleanPath(p)
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	addPath(d.RemotePath)
	for _, line := range strings.Split(d.RemotePaths, "\n") {
		addPath(line)
	}
	return paths
}

func (d *Chunker) setChunkNameFormat(pattern string) error {
	if dir, _ := path.Split(pattern); dir != "" {
		return errors.New("directory separator prohibited")
	}

	nameStart, nameEnd, err := parseNameToken(pattern)
	if err != nil {
		return err
	}
	chunkStart, chunkEnd, chunkWidth, err := parseChunkToken(pattern)
	if err != nil {
		return err
	}
	if nameStart > chunkStart {
		return errors.New("name token must come before chunk token")
	}

	reDigits := "[0-9]+"
	if chunkWidth > 0 {
		reDigits = fmt.Sprintf("[0-9]{%d,}", chunkWidth)
	}
	reDataOrCtrl := fmt.Sprintf("(?:(%s)|_(%s))", reDigits, ctrlTypeRegStr)

	beforeName := pattern[:nameStart]
	between := pattern[nameEnd:chunkStart]
	afterChunk := pattern[chunkEnd:]

	strRegex := fmt.Sprintf(
		"^%s(.+?)%s%s%s(?:%s|%s)?$",
		regexp.QuoteMeta(beforeName),
		regexp.QuoteMeta(between),
		reDataOrCtrl,
		regexp.QuoteMeta(afterChunk),
		tempSuffixRegStr,
		tempSuffixRegOld,
	)
	d.nameRegexp = regexp.MustCompile(strRegex)

	fmtDigits := "%d"
	if chunkWidth > 0 {
		fmtDigits = fmt.Sprintf("%%0%dd", chunkWidth)
	}
	d.dataNameFmt = strings.ReplaceAll(beforeName, "%", "%%") +
		"%s" +
		strings.ReplaceAll(between, "%", "%%") +
		fmtDigits +
		strings.ReplaceAll(afterChunk, "%", "%%")
	return nil
}

func parseNameToken(pattern string) (start, end int, err error) {
	nameMagicCount := strings.Count(pattern, "{name}")
	switch nameMagicCount {
	case 0:
		return 0, 0, errors.New("pattern must contain one name token: {name}")
	case 1:
	default:
		return 0, 0, errors.New("pattern must contain exactly one name token: {name}")
	}
	start = strings.Index(pattern, "{name}")
	return start, start + len("{name}"), nil
}

func parseChunkToken(pattern string) (start, end, width int, err error) {
	chunkMatches := chunkTokenRegexp.FindAllStringSubmatchIndex(pattern, -1)
	switch len(chunkMatches) {
	case 0:
		return 0, 0, 0, errors.New("pattern must contain one chunk token: {chunk} or {chunk:N}")
	case 1:
	default:
		return 0, 0, 0, errors.New("pattern must contain exactly one chunk token: {chunk} or {chunk:N}")
	}
	match := chunkMatches[0]
	start = match[0]
	end = match[1]
	if match[2] >= 0 && match[3] >= 0 {
		width, err = strconv.Atoi(pattern[match[2]:match[3]])
		if err != nil || width <= 0 {
			return 0, 0, 0, errors.New("chunk width in {chunk:N} must be a positive integer")
		}
	}
	return start, end, width, nil
}

func (d *Chunker) makeChunkName(filePath string, chunkNo int, xactID string) string {
	dir, baseName := path.Split(filePath)
	name := fmt.Sprintf(d.dataNameFmt, baseName, chunkNo+d.StartFrom)
	if xactID != "" {
		name += fmt.Sprintf(tempSuffixFormat, xactID)
	}
	return dir + name
}

func (d *Chunker) parseChunkName(filePath string) (parentPath string, chunkNo int, ctrlType, xactID string) {
	dir, name := path.Split(filePath)
	match := d.nameRegexp.FindStringSubmatch(name)
	if match == nil || match[1] == "" {
		return "", -1, "", ""
	}

	chunkNo = -1
	if match[2] != "" {
		n, err := strconv.Atoi(match[2])
		if err != nil {
			return "", -1, "", ""
		}
		chunkNo = n - d.StartFrom
		if chunkNo < 0 {
			return "", -1, "", ""
		}
	}

	if match[4] != "" {
		xactID = match[4]
	}
	if match[5] != "" {
		oldNum, err := strconv.ParseInt(match[5], 10, 64)
		if err != nil || oldNum < 0 {
			return "", -1, "", ""
		}
		xactID = fmt.Sprintf(tempSuffixFormat, strconv.FormatInt(oldNum, 36))[1:]
	}

	return dir + match[1], chunkNo, match[3], xactID
}

func marshalMetadata(size int64, nChunks int, md5Value, sha1Value, xactID string) ([]byte, error) {
	version := chunkerMetadataVerion
	if xactID == "" && version == 2 {
		version = 1
	}
	meta := metadataJSON{
		Version:  &version,
		Size:     &size,
		ChunkNum: &nChunks,
		MD5:      md5Value,
		SHA1:     sha1Value,
		XactID:   xactID,
	}
	data, err := json.Marshal(&meta)
	if err == nil && len(data) >= maxMetadataSizeWrite {
		return nil, errors.New("metadata can't be this big")
	}
	return data, err
}

func unmarshalMetadata(data []byte) (*chunkMetadata, error) {
	if len(data) > maxMetadataSizeWrite {
		return nil, errors.New("metadata is too large")
	}
	if data == nil || len(data) < 2 || data[0] != '{' || data[len(data)-1] != '}' {
		return nil, errors.New("invalid json")
	}
	var meta metadataJSON
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.Version == nil || meta.Size == nil || meta.ChunkNum == nil {
		return nil, errors.New("missing required field")
	}
	if *meta.Version < 1 {
		return nil, errors.New("wrong version")
	}
	if *meta.Size < 0 {
		return nil, errors.New("negative file size")
	}
	if *meta.ChunkNum < 1 || *meta.ChunkNum > maxSafeChunkNumber {
		return nil, errors.New("wrong number of chunks")
	}
	if meta.MD5 != "" {
		if _, err := hex.DecodeString(meta.MD5); err != nil || len(meta.MD5) != 32 {
			return nil, errors.New("wrong md5 hash")
		}
	}
	if meta.SHA1 != "" {
		if _, err := hex.DecodeString(meta.SHA1); err != nil || len(meta.SHA1) != 40 {
			return nil, errors.New("wrong sha1 hash")
		}
	}
	if *meta.Version > chunkerMetadataVerion {
		return nil, errors.New("unknown metadata version")
	}
	return &chunkMetadata{
		Version: *meta.Version,
		Size:    *meta.Size,
		NChunks: *meta.ChunkNum,
		MD5:     meta.MD5,
		SHA1:    meta.SHA1,
		XactID:  meta.XactID,
	}, nil
}

func joinRemotePathWithBase(baseMountPath, logicalPath string) string {
	logicalPath = utils.FixAndCleanPath(logicalPath)
	if utils.PathEqual(logicalPath, "/") {
		return utils.FixAndCleanPath(baseMountPath)
	}
	return path.Join(utils.FixAndCleanPath(baseMountPath), logicalPath)
}

func (d *Chunker) joinRemotePath(logicalPath string) string {
	return joinRemotePathWithBase(d.RemotePath, logicalPath)
}

func (d *Chunker) joinRemotePathForTarget(logicalPath string, remoteIndex int) string {
	target := d.remoteTargets[remoteIndex]
	return joinRemotePathWithBase(target.MountPath, logicalPath)
}

func (d *Chunker) getActualPathForRemote(logicalPath string) (string, error) {
	return d.getActualPathForRemoteOnTarget(logicalPath, 0)
}

func (d *Chunker) getActualPathForRemoteOnTarget(logicalPath string, remoteIndex int) (string, error) {
	_, actualPath, err := op.GetStorageAndActualPath(d.joinRemotePathForTarget(logicalPath, remoteIndex))
	return actualPath, err
}

func (d *Chunker) getActualChunkPath(filePath string, chunkNo int, xactID string, remoteIndex int) (string, error) {
	return d.getActualPathForRemoteOnTarget(d.makeChunkName(filePath, chunkNo, xactID), remoteIndex)
}

func (d *Chunker) chunkTargetIndex(chunkNo int) int {
	targetIndexes := d.chunkTargetIndexes()
	if len(targetIndexes) == 0 {
		return 0
	}
	if chunkNo < 0 {
		return targetIndexes[0]
	}
	return targetIndexes[chunkNo%len(targetIndexes)]
}

func (d *Chunker) chunkTargetIndexes() []int {
	if len(d.remoteTargets) <= 1 {
		return []int{0}
	}
	if d.StoreChunksInPrimary {
		targets := make([]int, 0, len(d.remoteTargets))
		for i := range d.remoteTargets {
			targets = append(targets, i)
		}
		return targets
	}
	targets := make([]int, 0, len(d.remoteTargets)-1)
	for i := 1; i < len(d.remoteTargets); i++ {
		targets = append(targets, i)
	}
	if len(targets) == 0 {
		return []int{0}
	}
	return targets
}

func (d *Chunker) targetLocation(logicalPath string, remoteIndex int) objectLocation {
	return objectLocation{
		LogicalPath: utils.FixAndCleanPath(logicalPath),
		RemoteIndex: remoteIndex,
	}
}

func (d *Chunker) chunkLocation(filePath string, part chunkPart) objectLocation {
	return d.targetLocation(d.makeChunkName(filePath, part.No, part.XactID), part.RemoteIndex)
}

func (d *Chunker) listDirObjects(ctx context.Context, dirPath string, refresh bool) ([]model.Obj, error) {
	groups := map[string]*groupInfo{}
	dirMap := map[string]model.Obj{}
	found := false

	for remoteIndex := range d.remoteTargets {
		remotePath := d.joinRemotePathForTarget(dirPath, remoteIndex)
		entries, err := fsList(ctx, remotePath, refresh)
		if err != nil {
			if errs.IsObjectNotFound(err) {
				continue
			}
			return nil, err
		}
		found = true

		for _, entry := range entries {
			if entry.IsDir() {
				if _, ok := dirMap[entry.GetName()]; !ok {
					dirMap[entry.GetName()] = &model.Object{
						Name:     entry.GetName(),
						Path:     path.Join(dirPath, entry.GetName()),
						Size:     0,
						Modified: entry.ModTime(),
						Ctime:    entry.CreateTime(),
						IsFolder: true,
						HashInfo: entry.GetHash(),
					}
				}
				continue
			}

			mainName, chunkNo, ctrlType, xactID := d.parseChunkName(entry.GetName())
			if mainName == "" {
				g := groups[entry.GetName()]
				if g == nil {
					g = &groupInfo{partsByXact: map[string]map[int]chunkPart{}}
					groups[entry.GetName()] = g
				}
				if g.base == nil || remoteIndex < g.base.RemoteIndex {
					g.base = &locatedObj{
						Obj:         entry,
						RemoteIndex: remoteIndex,
					}
				}
				continue
			}
			if chunkNo < 0 || ctrlType != "" {
				continue
			}
			g := groups[mainName]
			if g == nil {
				g = &groupInfo{partsByXact: map[string]map[int]chunkPart{}}
				groups[mainName] = g
			}
			if g.partsByXact[xactID] == nil {
				g.partsByXact[xactID] = map[int]chunkPart{}
			}
			part := chunkPart{
				No:          chunkNo,
				Size:        entry.GetSize(),
				XactID:      xactID,
				RemoteIndex: remoteIndex,
			}
			existing, ok := g.partsByXact[xactID][chunkNo]
			if !ok || part.RemoteIndex < existing.RemoteIndex {
				g.partsByXact[xactID][chunkNo] = part
			}
		}
	}

	if !found && !utils.PathEqual(dirPath, "/") {
		return nil, errs.ObjectNotFound
	}

	dirs := make([]model.Obj, 0, len(dirMap))
	for _, obj := range dirMap {
		dirs = append(dirs, obj)
	}

	result := make([]model.Obj, 0, len(dirs)+len(groups))
	result = append(result, dirs...)
	for name, group := range groups {
		obj, ok, err := d.buildListedObject(ctx, dirPath, name, group)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, obj)
		}
	}
	return result, nil
}

func (d *Chunker) targetPathExists(ctx context.Context, remoteIndex int, logicalPath string) (bool, error) {
	_, err := fs.Get(ctx, d.joinRemotePathForTarget(logicalPath, remoteIndex), &fs.GetArgs{NoLog: true})
	if err == nil {
		return true, nil
	}
	if errs.IsObjectNotFound(err) {
		return false, nil
	}
	return false, err
}

func (d *Chunker) ensureDirOnTarget(ctx context.Context, remoteIndex int, logicalDirPath string) error {
	logicalDirPath = utils.FixAndCleanPath(logicalDirPath)
	if utils.PathEqual(logicalDirPath, "/") {
		return nil
	}
	return fs.MakeDir(ctx, d.joinRemotePathForTarget(logicalDirPath, remoteIndex))
}

func (d *Chunker) ensureDirOnAllTargets(ctx context.Context, logicalDirPath string) error {
	var errsList []error
	for remoteIndex := range d.remoteTargets {
		if err := d.ensureDirOnTarget(ctx, remoteIndex, logicalDirPath); err != nil {
			errsList = append(errsList, err)
		}
	}
	return errors.Join(errsList...)
}

func (d *Chunker) existingLocationsForDir(ctx context.Context, logicalDirPath string) ([]int, error) {
	locations := make([]int, 0, len(d.remoteTargets))
	for remoteIndex := range d.remoteTargets {
		exists, err := d.targetPathExists(ctx, remoteIndex, logicalDirPath)
		if err != nil {
			return nil, err
		}
		if exists {
			locations = append(locations, remoteIndex)
		}
	}
	return locations, nil
}

func (d *Chunker) dirLocationsOrAll(ctx context.Context, logicalDirPath string) ([]int, error) {
	locations, err := d.existingLocationsForDir(ctx, logicalDirPath)
	if err != nil {
		return nil, err
	}
	if len(locations) > 0 {
		return locations, nil
	}
	all := make([]int, 0, len(d.remoteTargets))
	for remoteIndex := range d.remoteTargets {
		all = append(all, remoteIndex)
	}
	return all, nil
}

func (d *Chunker) moveDirAcrossTargets(ctx context.Context, srcPath, dstDirPath string) error {
	locations, err := d.existingLocationsForDir(ctx, srcPath)
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return errs.ObjectNotFound
	}
	var errsList []error
	for _, remoteIndex := range locations {
		if err := d.ensureDirOnTarget(ctx, remoteIndex, dstDirPath); err != nil {
			errsList = append(errsList, err)
			continue
		}
		srcActualPath, err := d.getActualPathForRemoteOnTarget(srcPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		dstActualPath, err := d.getActualPathForRemoteOnTarget(dstDirPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		if err := op.Move(ctx, d.remoteTargets[remoteIndex].Storage, srcActualPath, dstActualPath); err != nil {
			errsList = append(errsList, err)
		}
	}
	return errors.Join(errsList...)
}

func (d *Chunker) copyDirAcrossTargets(ctx context.Context, srcPath, dstDirPath string) error {
	locations, err := d.dirLocationsOrAll(ctx, srcPath)
	if err != nil {
		return err
	}
	var errsList []error
	for _, remoteIndex := range locations {
		exists, err := d.targetPathExists(ctx, remoteIndex, srcPath)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		if !exists {
			continue
		}
		if err := d.ensureDirOnTarget(ctx, remoteIndex, dstDirPath); err != nil {
			errsList = append(errsList, err)
			continue
		}
		srcActualPath, err := d.getActualPathForRemoteOnTarget(srcPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		dstActualPath, err := d.getActualPathForRemoteOnTarget(dstDirPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		if err := op.Copy(ctx, d.remoteTargets[remoteIndex].Storage, srcActualPath, dstActualPath); err != nil {
			errsList = append(errsList, err)
		}
	}
	return errors.Join(errsList...)
}

func (d *Chunker) renameDirAcrossTargets(ctx context.Context, srcPath, newName string) error {
	locations, err := d.existingLocationsForDir(ctx, srcPath)
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return errs.ObjectNotFound
	}
	var errsList []error
	for _, remoteIndex := range locations {
		srcActualPath, err := d.getActualPathForRemoteOnTarget(srcPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		if err := op.Rename(ctx, d.remoteTargets[remoteIndex].Storage, srcActualPath, newName); err != nil {
			errsList = append(errsList, err)
		}
	}
	return errors.Join(errsList...)
}

func (d *Chunker) removeDirAcrossTargets(ctx context.Context, logicalPath string) error {
	locations, err := d.existingLocationsForDir(ctx, logicalPath)
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return errs.ObjectNotFound
	}
	var errsList []error
	for _, remoteIndex := range locations {
		actualPath, err := d.getActualPathForRemoteOnTarget(logicalPath, remoteIndex)
		if err != nil {
			errsList = append(errsList, err)
			continue
		}
		if err := op.Remove(ctx, d.remoteTargets[remoteIndex].Storage, actualPath); err != nil {
			errsList = append(errsList, err)
		}
	}
	return errors.Join(errsList...)
}

func (d *Chunker) buildListedObject(ctx context.Context, dirPath, name string, group *groupInfo) (model.Obj, bool, error) {
	var meta *chunkMetadata
	var err error
	if group.base != nil && group.base.Obj.GetSize() <= maxMetadataSizeRead && len(group.partsByXact) > 0 {
		meta, err = d.readMetadata(ctx, path.Join(dirPath, name), group.base.Obj.GetSize(), group.base.RemoteIndex)
		if err != nil {
			meta = nil
		}
	}

	if meta == nil && group.base != nil && len(group.partsByXact) == 0 {
		return &Object{
			Object: model.Object{
				Name:     name,
				Path:     path.Join(dirPath, name),
				Size:     group.base.Obj.GetSize(),
				Modified: group.base.Obj.ModTime(),
				Ctime:    group.base.Obj.CreateTime(),
				IsFolder: false,
				HashInfo: group.base.Obj.GetHash(),
			},
			Main:            group.base.Obj,
			MainRemoteIndex: group.base.RemoteIndex,
		}, true, nil
	}

	selected := map[int]chunkPart{}
	switch {
	case meta != nil:
		selected = group.partsByXact[meta.XactID]
	case group.base == nil:
		selected = group.partsByXact[""]
	default:
		return &Object{
			Object: model.Object{
				Name:     name,
				Path:     path.Join(dirPath, name),
				Size:     group.base.Obj.GetSize(),
				Modified: group.base.Obj.ModTime(),
				Ctime:    group.base.Obj.CreateTime(),
				IsFolder: false,
				HashInfo: group.base.Obj.GetHash(),
			},
			Main:            group.base.Obj,
			MainRemoteIndex: group.base.RemoteIndex,
		}, true, nil
	}

	parts := sortChunkParts(selected)
	if len(parts) == 0 {
		if meta != nil {
			return &Object{
				Object: model.Object{
					Name:     name,
					Path:     path.Join(dirPath, name),
					Size:     meta.Size,
					Modified: group.base.Obj.ModTime(),
					Ctime:    group.base.Obj.CreateTime(),
					IsFolder: false,
					HashInfo: buildHashInfo(meta),
				},
				Main:            group.base.Obj,
				MainRemoteIndex: group.base.RemoteIndex,
				Meta:            meta,
				Chunked:         true,
				UsesMeta:        true,
			}, true, nil
		}
		return nil, false, nil
	}

	size := int64(0)
	for _, part := range parts {
		size += part.Size
	}
	modified := time.Time{}
	ctime := time.Time{}
	if group.base != nil {
		modified = group.base.Obj.ModTime()
		ctime = group.base.Obj.CreateTime()
	}
	if meta != nil {
		size = meta.Size
	}

	mainRemoteIndex := 0
	var mainObj model.Obj
	if group.base != nil {
		mainRemoteIndex = group.base.RemoteIndex
		mainObj = group.base.Obj
	}

	return &Object{
		Object: model.Object{
			Name:     name,
			Path:     path.Join(dirPath, name),
			Size:     size,
			Modified: modified,
			Ctime:    ctime,
			IsFolder: false,
			HashInfo: buildHashInfo(meta),
		},
		Main:            mainObj,
		MainRemoteIndex: mainRemoteIndex,
		Parts:           parts,
		Meta:            meta,
		Chunked:         true,
		UsesMeta:        meta != nil,
	}, true, nil
}

func (d *Chunker) readMetadata(ctx context.Context, logicalPath string, size int64, remoteIndex int) (*chunkMetadata, error) {
	actualPath, err := d.getActualPathForRemoteOnTarget(logicalPath, remoteIndex)
	if err != nil {
		return nil, err
	}
	link, obj, err := op.Link(ctx, d.remoteTargets[remoteIndex].Storage, actualPath, model.LinkArgs{})
	if err != nil {
		return nil, err
	}
	ss, err := stream.NewSeekableStream(stream.FileStream{Ctx: ctx, Obj: obj}, link)
	if err != nil {
		return nil, err
	}
	defer ss.Close()

	reader, err := ss.RangeRead(http_range.Range{Start: 0, Length: size})
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return unmarshalMetadata(data)
}

func sortChunkParts(parts map[int]chunkPart) []chunkPart {
	result := make([]chunkPart, 0, len(parts))
	for _, part := range parts {
		result = append(result, part)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].No < result[j].No
	})
	return result
}

func buildHashInfo(meta *chunkMetadata) utils.HashInfo {
	if meta == nil {
		return utils.HashInfo{}
	}
	hashes := map[*utils.HashType]string{}
	if meta.MD5 != "" {
		hashes[utils.MD5] = meta.MD5
	}
	if meta.SHA1 != "" {
		hashes[utils.SHA1] = meta.SHA1
	}
	return utils.NewHashInfoByMap(hashes)
}

func (d *Chunker) linkedObject(obj model.Obj) *Object {
	if linked, ok := obj.(*Object); ok {
		return linked
	}
	return nil
}

func (d *Chunker) objectLocationsForObject(obj *Object) []objectLocation {
	if obj == nil {
		return nil
	}
	locations := make([]objectLocation, 0, len(obj.Parts)+1)
	if obj.Chunked && obj.UsesMeta {
		locations = append(locations, d.targetLocation(obj.GetPath(), obj.MainRemoteIndex))
	}
	if !obj.Chunked {
		locations = append(locations, d.targetLocation(obj.GetPath(), obj.MainRemoteIndex))
		return locations
	}
	for _, part := range obj.Parts {
		locations = append(locations, d.chunkLocation(obj.GetPath(), part))
	}
	return locations
}

func (d *Chunker) cleanupReplacedObject(ctx context.Context, obj *Object, keep map[string]struct{}) error {
	if obj == nil {
		return nil
	}
	var errs []error
	for _, location := range d.objectLocationsForObject(obj) {
		if _, ok := keep[d.keepKey(location)]; ok {
			continue
		}
		actualPath, err := d.getActualPathForRemoteOnTarget(location.LogicalPath, location.RemoteIndex)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := op.Remove(ctx, d.remoteTargets[location.RemoteIndex].Storage, actualPath); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (d *Chunker) keepKey(location objectLocation) string {
	return fmt.Sprintf("%d:%s", location.RemoteIndex, utils.FixAndCleanPath(location.LogicalPath))
}

func (d *Chunker) buildKeepSet(locations ...objectLocation) map[string]struct{} {
	keep := make(map[string]struct{}, len(locations))
	for _, location := range locations {
		if location.LogicalPath == "" {
			continue
		}
		keep[d.keepKey(location)] = struct{}{}
	}
	return keep
}

func (d *Chunker) chunkPartBaseName(filePath string, chunkNo int, xactID string) string {
	return path.Base(d.makeChunkName(filePath, chunkNo, xactID))
}

func (d *Chunker) openChunkReader(ctx context.Context, parts []linkedPart, totalSize int64, req http_range.Range) (io.ReadCloser, error) {
	if req.Start < 0 || req.Start > totalSize {
		return nil, fmt.Errorf("range start out of bound")
	}
	if req.Length < 0 || req.Start+req.Length > totalSize {
		req.Length = totalSize - req.Start
	}
	if req.Length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}

	var (
		readers   []io.Reader
		closers   = utils.EmptyClosers()
		offset    int64
		remaining = req.Length
	)
	for _, part := range parts {
		partStart := offset
		partEnd := offset + part.part.Size
		offset = partEnd
		if req.Start >= partEnd || remaining <= 0 {
			continue
		}
		localStart := int64(0)
		if req.Start > partStart {
			localStart = req.Start - partStart
		}
		localLength := utils.Min(part.part.Size-localStart, remaining)
		rc, err := d.openPartRange(ctx, part.link, part.part.Size, localStart, localLength)
		if err != nil {
			_ = closers.Close()
			return nil, err
		}
		readers = append(readers, rc)
		closers.Add(rc)
		remaining -= localLength
	}
	if remaining > 0 {
		_ = closers.Close()
		return nil, errors.New("missing chunk data")
	}
	return utils.NewReadCloser(io.MultiReader(readers...), func() error {
		return closers.Close()
	}), nil
}

func (d *Chunker) openPartRange(ctx context.Context, link *model.Link, size, offset, length int64) (io.ReadCloser, error) {
	httpRange := http_range.Range{Start: offset, Length: length}
	switch {
	case link.MFile != nil:
		return io.NopCloser(io.NewSectionReader(link.MFile, offset, length)), nil
	case link.RangeReadCloser != nil:
		return link.RangeReadCloser.RangeRead(ctx, httpRange)
	case link.URL != "":
		rrc, err := stream.GetRangeReadCloserFromLink(size, link)
		if err != nil {
			return nil, err
		}
		rc, err := rrc.RangeRead(ctx, httpRange)
		if err != nil {
			_ = rrc.Close()
			return nil, err
		}
		return utils.NewReadCloser(rc, func() error {
			return rrc.Close()
		}), nil
	default:
		return nil, errors.New("chunk part has no readable link")
	}
}

func fsList(ctx context.Context, remotePath string, refresh bool) ([]model.Obj, error) {
	return fs.List(ctx, remotePath, &fs.ListArgs{NoLog: true, Refresh: refresh})
}
