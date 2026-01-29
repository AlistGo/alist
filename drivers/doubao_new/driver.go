package doubao_new

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type DoubaoNew struct {
	model.Storage
	Addition
	TtLogid string
}

func (d *DoubaoNew) Config() driver.Config {
	return config
}

func (d *DoubaoNew) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *DoubaoNew) Init(ctx context.Context) error {
	// TODO login / refresh token
	//op.MustSaveDriverStorage(d)
	return nil
}

func (d *DoubaoNew) Drop(ctx context.Context) error {
	return nil
}

func (d *DoubaoNew) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	nodes, err := d.listAllChildren(ctx, dir.GetID())
	if err != nil {
		return nil, err
	}

	objs := make([]model.Obj, 0, len(nodes))
	for _, node := range nodes {
		size := parseSize(node.Extra.Size)
		isFolder := node.Type == 0
		obj := &Object{
			Object: model.Object{
				ID:       node.NodeToken,
				Path:     dir.GetID(),
				Name:     node.Name,
				Size:     size,
				Modified: time.Unix(node.EditTime, 0),
				Ctime:    time.Unix(node.CreateTime, 0),
				IsFolder: isFolder,
			},
			ObjToken: node.ObjToken,
			NodeType: node.NodeType,
			ObjType:  node.Type,
			URL:      node.URL,
		}
		objs = append(objs, obj)
	}

	return objs, nil
}

func (d *DoubaoNew) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	obj, ok := file.(*Object)
	if !ok {
		return nil, errors.New("unsupported object type")
	}
	if obj.IsFolder {
		return nil, errs.LinkIsDir
	}
	if args.Type == "preview" || args.Type == "thumb" {
		if link, err := d.previewLink(ctx, obj, args); err == nil {
			return link, nil
		}
	}
	auth := d.resolveAuthorization()
	dpop := d.resolveDpop()
	if auth == "" || dpop == "" {
		return nil, errors.New("missing authorization or dpop")
	}
	if obj.ObjToken == "" {
		return nil, errors.New("missing obj_token")
	}

	query := url.Values{}
	query.Set("authorization", auth)
	query.Set("dpop", dpop)

	downloadURL := DownloadBaseURL + "/space/api/box/stream/download/all/" + obj.ObjToken + "/?" + query.Encode()

	headers := http.Header{
		"Referer":    []string{"https://www.doubao.com/"},
		"User-Agent": []string{base.UserAgent},
	}

	return &model.Link{
		URL:    downloadURL,
		Header: headers,
	}, nil
}

func (d *DoubaoNew) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	node, err := d.createFolder(ctx, parentDir.GetID(), dirName)
	if err != nil {
		return nil, err
	}
	return &Object{
		Object: model.Object{
			ID:       node.NodeToken,
			Path:     parentDir.GetID(),
			Name:     node.Name,
			Size:     parseSize(node.Extra.Size),
			Modified: time.Unix(node.EditTime, 0),
			Ctime:    time.Unix(node.CreateTime, 0),
			IsFolder: true,
		},
		ObjToken: node.ObjToken,
		NodeType: node.NodeType,
		ObjType:  node.Type,
		URL:      node.URL,
	}, nil
}

func (d *DoubaoNew) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if srcObj == nil {
		return nil, errors.New("nil source object")
	}
	if dstDir == nil {
		return nil, errors.New("nil destination dir")
	}
	srcToken := srcObj.GetID()
	if srcToken == "" {
		if obj, ok := srcObj.(*Object); ok {
			srcToken = obj.ObjToken
		}
	}
	if srcToken == "" {
		return nil, errors.New("missing source token")
	}
	if err := d.moveObj(ctx, srcToken, dstDir.GetID()); err != nil {
		return nil, err
	}
	if obj, ok := srcObj.(*Object); ok {
		clone := *obj
		clone.Path = dstDir.GetID()
		return &clone, nil
	}
	return srcObj, nil
}

func (d *DoubaoNew) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	if srcObj == nil {
		return nil, errors.New("nil source object")
	}
	if srcObj.IsDir() {
		if err := d.renameFolder(ctx, srcObj.GetID(), newName); err != nil {
			return nil, err
		}
	} else {
		fileToken := ""
		if obj, ok := srcObj.(*Object); ok {
			fileToken = obj.ObjToken
		}
		if fileToken == "" {
			fileToken = srcObj.GetID()
		}
		if err := d.renameFile(ctx, fileToken, newName); err != nil {
			return nil, err
		}
	}

	if obj, ok := srcObj.(*Object); ok {
		clone := *obj
		clone.Name = newName
		return &clone, nil
	}
	return srcObj, nil
}

func (d *DoubaoNew) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	// TODO copy obj, optional
	return nil, errs.NotImplement
}

func (d *DoubaoNew) Remove(ctx context.Context, obj model.Obj) error {
	if obj == nil {
		return errors.New("nil object")
	}
	token := obj.GetID()
	if token == "" {
		if o, ok := obj.(*Object); ok {
			token = o.ObjToken
		}
	}
	if token == "" {
		return errors.New("missing object token")
	}
	return d.removeObj(ctx, []string{token})
}

func (d *DoubaoNew) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	if file == nil {
		return nil, errors.New("nil file")
	}
	if file.GetSize() <= 0 {
		return nil, errors.New("invalid file size")
	}

	uploadPrep, err := d.prepareUpload(ctx, file.GetName(), file.GetSize(), dstDir.GetID())
	if err != nil {
		return nil, err
	}
	if uploadPrep.BlockSize <= 0 {
		return nil, errors.New("invalid block size from prepare")
	}

	tmpFile, err := file.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}
	defer tmpFile.Close()

	blockSize := uploadPrep.BlockSize
	totalSize := file.GetSize()
	numBlocks := int((totalSize + blockSize - 1) / blockSize)
	blocks := make([]UploadBlock, 0, numBlocks)
	blockMeta := make(map[int]UploadBlock, numBlocks)

	for seq := 0; seq < numBlocks; seq++ {
		offset := int64(seq) * blockSize
		length := blockSize
		if remain := totalSize - offset; remain < length {
			length = remain
		}
		buf := make([]byte, int(length))
		n, err := tmpFile.ReadAt(buf, offset)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		buf = buf[:n]
		sum := sha256.Sum256(buf)
		hash := base64.StdEncoding.EncodeToString(sum[:])
		checksum := adler32String(buf)

		block := UploadBlock{
			Hash:       hash,
			Seq:        seq,
			Size:       int64(n),
			Checksum:   checksum,
			IsUploaded: true,
		}
		blocks = append(blocks, block)
		blockMeta[seq] = block
	}

	needed, err := d.uploadBlocks(ctx, uploadPrep.UploadID, blocks, "explorer")
	if err != nil {
		return nil, err
	}

	if len(needed.NeededUploadBlocks) > 0 {
		sort.Slice(needed.NeededUploadBlocks, func(i, j int) bool {
			return needed.NeededUploadBlocks[i].Seq < needed.NeededUploadBlocks[j].Seq
		})
		const maxMergeBlockCount = 20
		var (
			groupSeqs      []int
			groupChecksums []string
			groupSizes     []int64
			groupRealSize  int64
			groupExpectSum int64
			groupBuf       bytes.Buffer
			uploadedBytes  int64
		)

		flushGroup := func() error {
			if len(groupSeqs) == 0 {
				return nil
			}
			data := groupBuf.Bytes()
			expectLen := groupExpectSum
			if len(data) > 0 {
				headLen := 32
				if len(data) < headLen {
					headLen = len(data)
				}
				tailLen := 32
				if len(data) < tailLen {
					tailLen = len(data)
				}
				fmt.Printf("head32 = %x\n", data[:headLen])
				fmt.Printf("tail32 = %x\n", data[len(data)-tailLen:])
			}
			if int64(len(data)) != expectLen {
				return fmt.Errorf("[doubao_new] merge blocks invalid body len: got=%d expect=%d seqs=%v", len(data), expectLen, groupSeqs)
			}
			mergeResp, err := d.mergeUploadBlocks(ctx, uploadPrep.UploadID, groupSeqs, groupChecksums, groupSizes, blockSize, data)
			if err != nil {
				return err
			}
			if len(mergeResp.SuccessSeqList) != len(groupSeqs) {
				return fmt.Errorf("[doubao_new] merge blocks incomplete: %v", mergeResp.SuccessSeqList)
			}
			success := make(map[int]bool, len(mergeResp.SuccessSeqList))
			for _, seq := range mergeResp.SuccessSeqList {
				success[seq] = true
			}
			for _, seq := range groupSeqs {
				if !success[seq] {
					return fmt.Errorf("[doubao_new] merge blocks missing seq %d", seq)
				}
			}

			uploadedBytes += groupRealSize
			groupSeqs = groupSeqs[:0]
			groupChecksums = groupChecksums[:0]
			groupSizes = groupSizes[:0]
			groupRealSize = 0
			groupExpectSum = 0
			groupBuf.Reset()
			if up != nil {
				percent := float64(uploadedBytes) / float64(totalSize) * 100
				up(percent)
			}
			return nil
		}

		for _, item := range needed.NeededUploadBlocks {
			if _, ok := blockMeta[item.Seq]; !ok {
				return nil, fmt.Errorf("[doubao_new] missing block meta for seq %d", item.Seq)
			}
			if item.Size <= 0 {
				return nil, fmt.Errorf("[doubao_new] invalid block size from needed list: seq=%d size=%d", item.Seq, item.Size)
			}
			offset := int64(item.Seq) * blockSize
			buf := make([]byte, int(item.Size))
			n, err := tmpFile.ReadAt(buf, offset)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return nil, err
			}
			if n != len(buf) {
				return nil, fmt.Errorf("[doubao_new] short read: seq=%d want=%d got=%d", item.Seq, len(buf), n)
			}
			buf = buf[:n]
			realAdler := adler32String(buf)
			if realAdler != item.Checksum {
				return nil, fmt.Errorf("[doubao_new] block checksum mismatch: seq=%d offset=%d adler32=%s step2=%s", item.Seq, offset, realAdler, item.Checksum)
			}
			payloadStart := groupBuf.Len()
			groupBuf.Write(buf)
			payloadEnd := groupBuf.Len()
			payloadAdler := adler32String(groupBuf.Bytes()[payloadStart:payloadEnd])
			if payloadAdler != item.Checksum {
				return nil, fmt.Errorf("[doubao_new] payload checksum mismatch: seq=%d start=%d end=%d adler32=%s step2=%s", item.Seq, payloadStart, payloadEnd, payloadAdler, item.Checksum)
			}
			groupSeqs = append(groupSeqs, item.Seq)
			groupChecksums = append(groupChecksums, item.Checksum)
			groupSizes = append(groupSizes, item.Size)
			groupRealSize += int64(n)
			groupExpectSum += item.Size
			if len(groupSeqs) >= maxMergeBlockCount {
				if err := flushGroup(); err != nil {
					return nil, err
				}
			}
		}

		if err := flushGroup(); err != nil {
			return nil, err
		}
		if up != nil {
			up(100)
		}
	} else if up != nil {
		up(100)
	}

	numBlocksFinish := uploadPrep.NumBlocks
	if numBlocksFinish <= 0 {
		numBlocksFinish = numBlocks
	}
	finish, err := d.finishUpload(ctx, uploadPrep.UploadID, numBlocksFinish, "explorer")
	if err != nil {
		return nil, err
	}

	nodeToken := finish.Extra.NodeToken
	if nodeToken == "" {
		nodeToken = finish.FileToken
	}
	now := time.Now()
	return &Object{
		Object: model.Object{
			ID:       nodeToken,
			Path:     dstDir.GetID(),
			Name:     file.GetName(),
			Size:     file.GetSize(),
			Modified: now,
			Ctime:    now,
			IsFolder: false,
		},
		ObjToken: finish.FileToken,
	}, nil
}

func (d *DoubaoNew) GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	// TODO get archive file meta-info, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoNew) ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	// TODO list args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoNew) Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error) {
	// TODO return link of file args.InnerPath in the archive obj, return errs.NotImplement to use an internal archive tool, optional
	return nil, errs.NotImplement
}

func (d *DoubaoNew) ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error) {
	// TODO extract args.InnerPath path in the archive srcObj to the dstDir location, optional
	// a folder with the same name as the archive file needs to be created to store the extracted results if args.PutIntoNewDir
	// return errs.NotImplement to use an internal archive tool
	return nil, errs.NotImplement
}

func (d *DoubaoNew) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
	switch args.Method {
	case "doubao_preview", "preview":
		obj, ok := args.Obj.(*Object)
		if !ok {
			return nil, errors.New("unsupported object type")
		}
		info, err := d.getFileInfo(ctx, obj.ObjToken)
		if err != nil {
			return nil, err
		}
		entry, ok := info.PreviewMeta.Data["22"]
		if !ok || entry.Status != 0 {
			return nil, errs.NotSupport
		}

		imgExt := ".webp"
		pageNums := 1
		if entry.Extra != "" {
			var extra PreviewImageExtra
			if err := json.Unmarshal([]byte(entry.Extra), &extra); err == nil {
				if extra.ImgExt != "" {
					imgExt = extra.ImgExt
				}
				if extra.PageNums > 0 {
					pageNums = extra.PageNums
				}
			}
		}

		return base.Json{
			"version":   info.Version,
			"img_ext":   imgExt,
			"page_nums": pageNums,
		}, nil
	default:
		return nil, errs.NotSupport
	}
}

func (d *DoubaoNew) listAllChildren(ctx context.Context, parentToken string) ([]Node, error) {
	nodes := make([]Node, 0, 50)
	lastLabel := ""
	for page := 0; page < 100; page++ {
		data, err := d.listChildren(ctx, parentToken, lastLabel)
		if err != nil {
			return nil, err
		}

		if len(data.NodeList) > 0 {
			for _, token := range data.NodeList {
				node, ok := data.Entities.Nodes[token]
				if !ok {
					continue
				}
				nodes = append(nodes, node)
			}
		} else {
			for _, node := range data.Entities.Nodes {
				nodes = append(nodes, node)
			}
		}

		if !data.HasMore || data.LastLabel == "" || data.LastLabel == lastLabel {
			break
		}
		lastLabel = data.LastLabel
	}

	if len(nodes) == 0 {
		return nil, nil
	}
	return nodes, nil
}

func (d *DoubaoNew) previewLink(ctx context.Context, obj *Object, args model.LinkArgs) (*model.Link, error) {
	auth := d.resolveAuthorization()
	dpop := d.resolveDpop()
	if auth == "" || dpop == "" {
		return nil, errors.New("missing authorization or dpop")
	}
	if obj.ObjToken == "" {
		return nil, errors.New("missing obj_token")
	}
	info, err := d.getFileInfo(ctx, obj.ObjToken)
	if err != nil {
		return nil, err
	}

	entry, ok := info.PreviewMeta.Data["22"]
	if !ok || entry.Status != 0 {
		return nil, errors.New("preview not available")
	}

	subID := ""
	pageIndex := 0
	if args.HttpReq != nil {
		query := args.HttpReq.URL.Query()
		if v := query.Get("sub_id"); v != "" {
			subID = v
		} else if v := query.Get("page"); v != "" {
			if p, err := strconv.Atoi(v); err == nil && p >= 0 {
				pageIndex = p
			}
		}
	}
	if subID == "" {
		imgExt := ".webp"
		pageNums := 0
		if entry.Extra != "" {
			var extra PreviewImageExtra
			if err := json.Unmarshal([]byte(entry.Extra), &extra); err == nil {
				if extra.ImgExt != "" {
					imgExt = extra.ImgExt
				}
				pageNums = extra.PageNums
			}
		}
		if pageNums > 0 && pageIndex >= pageNums {
			pageIndex = pageNums - 1
		}
		subID = fmt.Sprintf("img_%d%s", pageIndex, imgExt)
	}

	query := url.Values{}
	query.Set("preview_type", "22")
	query.Set("sub_id", subID)
	if info.Version != "" {
		query.Set("version", info.Version)
	}
	previewURL := fmt.Sprintf("%s/space/api/box/stream/download/preview_sub/%s?%s", BaseURL, obj.ObjToken, query.Encode())

	headers := http.Header{
		"Referer":       []string{"https://www.doubao.com/"},
		"User-Agent":    []string{base.UserAgent},
		"Authorization": []string{auth},
		"Dpop":          []string{dpop},
	}

	return &model.Link{
		URL:    previewURL,
		Header: headers,
	}, nil
}

func parseSize(size string) int64 {
	if size == "" {
		return 0
	}
	val, err := strconv.ParseInt(size, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

var _ driver.Driver = (*DoubaoNew)(nil)
