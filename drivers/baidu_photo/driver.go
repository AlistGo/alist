package baiduphoto

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/errgroup"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/avast/retry-go"
	"github.com/go-resty/resty/v2"
)

type BaiduPhoto struct {
	model.Storage
	Addition

	// AccessToken string
	Uk       int64
	bdstoken string
	root     model.Obj

	uploadThread int
}

func (d *BaiduPhoto) Config() driver.Config {
	return config
}

func (d *BaiduPhoto) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *BaiduPhoto) Init(ctx context.Context) error {
	d.uploadThread, _ = strconv.Atoi(d.UploadThread)
	if d.uploadThread < 1 || d.uploadThread > 32 {
		d.uploadThread, d.UploadThread = 3, "3"
	}

	// if err := d.refreshToken(); err != nil {
	// 	return err
	// }

	// root
	if d.AlbumID != "" {
		albumID := strings.Split(d.AlbumID, "|")[0]
		album, err := d.GetAlbumDetail(ctx, albumID)
		if err != nil {
			return err
		}
		d.root = album
	} else {
		d.root = &Root{
			Name:     "root",
			Modified: d.Modified,
			IsFolder: true,
		}
	}

	// uk
	info, err := d.uInfo()
	if err != nil {
		return err
	}
	d.bdstoken, err = d.getBDStoken()
	if err != nil {
		return err
	}
	d.Uk, err = strconv.ParseInt(info.YouaID, 10, 64)
	return err
}

func (d *BaiduPhoto) GetRoot(ctx context.Context) (model.Obj, error) {
	return d.root, nil
}

func (d *BaiduPhoto) Drop(ctx context.Context) error {
	// d.AccessToken = ""
	d.Uk = 0
	d.root = nil
	return nil
}

func (d *BaiduPhoto) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	var err error

	/* album */
	if album, ok := dir.(*Album); ok {
		var files []AlbumFile
		files, err = d.GetAllAlbumFile(ctx, album, "")
		if err != nil {
			return nil, err
		}

		return utils.MustSliceConvert(files, func(file AlbumFile) model.Obj {
			return &file
		}), nil
	}

	/* root */
	var albums []Album
	if d.ShowType != "root_only_file" {
		albums, err = d.GetAllAlbum(ctx)
		if err != nil {
			return nil, err
		}
	}

	var files []File
	if d.ShowType != "root_only_album" {
		files, err = d.GetAllFile(ctx)
		if err != nil {
			return nil, err
		}
	}

	return append(
		utils.MustSliceConvert(albums, func(album Album) model.Obj {
			return &album
		}),
		utils.MustSliceConvert(files, func(album File) model.Obj {
			return &album
		})...,
	), nil

}

func (d *BaiduPhoto) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	switch file := file.(type) {
	case *File:
		return d.linkFile(ctx, file, args)
	case *AlbumFile:
		// 处理共享相册
		if d.Uk != file.Uk {
			// 有概率无法获取到链接
			// return d.linkAlbum(ctx, file, args)

			f, err := d.CopyAlbumFile(ctx, file)
			if err != nil {
				return nil, err
			}
			return d.linkFile(ctx, f, args)
		}
		return d.linkFile(ctx, &file.File, args)
	}
	return nil, errs.NotFile
}

var joinReg = regexp.MustCompile(`(?i)join:([\S]*)`)

func (d *BaiduPhoto) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	if _, ok := parentDir.(*Root); ok {
		code := joinReg.FindStringSubmatch(dirName)
		if len(code) > 1 {
			return d.JoinAlbum(ctx, code[1])
		}
		return d.CreateAlbum(ctx, dirName)
	}
	return nil, errs.NotSupport
}

func (d *BaiduPhoto) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	switch file := srcObj.(type) {
	case *File:
		if album, ok := dstDir.(*Album); ok {
			//rootfile ->  album
			return d.AddAlbumFile(ctx, album, file)
		}
	case *AlbumFile:
		switch album := dstDir.(type) {
		case *Root:
			//albumfile -> root
			return d.CopyAlbumFile(ctx, file)
		case *Album:
			// albumfile -> root -> album
			rootfile, err := d.CopyAlbumFile(ctx, file)
			if err != nil {
				return nil, err
			}
			return d.AddAlbumFile(ctx, album, rootfile)
		}
	}
	return nil, errs.NotSupport
}

func (d *BaiduPhoto) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	if file, ok := srcObj.(*AlbumFile); ok {
		switch dstDir.(type) {
		case *Album, *Root: // albumfile -> root -> album or albumfile -> root
			newObj, err := d.Copy(ctx, srcObj, dstDir)
			if err != nil {
				return nil, err
			}
			// 删除原相册文件
			_ = d.DeleteAlbumFile(ctx, file)
			return newObj, nil
		}
	}
	return nil, errs.NotSupport
}

func (d *BaiduPhoto) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	// 仅支持相册改名
	if album, ok := srcObj.(*Album); ok {
		return d.SetAlbumName(ctx, album, newName)
	}
	return nil, errs.NotSupport
}

func (d *BaiduPhoto) Remove(ctx context.Context, obj model.Obj) error {
	switch obj := obj.(type) {
	case *File:
		return d.DeleteFile(ctx, obj)
	case *AlbumFile:
		return d.DeleteAlbumFile(ctx, obj)
	case *Album:
		return d.DeleteAlbum(ctx, obj)
	}
	return errs.NotSupport
}

func (d *BaiduPhoto) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// 不支持大小为0的文件
	if stream.GetSize() == 0 {
		return nil, fmt.Errorf("file size cannot be zero")
	}

	// TODO:
	// 暂时没有找到妙传方式
	var (
		cache = stream.GetFile()
		tmpF  *os.File
		err   error
	)
	if _, ok := cache.(io.ReaderAt); !ok {
		tmpF, err = os.CreateTemp(conf.Conf.TempDir, "file-*")
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = tmpF.Close()
			_ = os.Remove(tmpF.Name())
		}()
		cache = tmpF
	}

	const DEFAULT int64 = 1 << 22
	const SliceSize int64 = 1 << 18

	// 计算需要的数据
	streamSize := stream.GetSize()
	count := int(streamSize / DEFAULT)
	lastBlockSize := streamSize % DEFAULT
	if lastBlockSize > 0 {
		count++
	} else {
		lastBlockSize = DEFAULT
	}

	// step.1 计算MD5
	sliceMD5List := make([]string, 0, count)
	byteSize := int64(DEFAULT)
	fileMd5H := md5.New()
	sliceMd5H := md5.New()
	sliceMd5H2 := md5.New()
	slicemd5H2Write := utils.LimitWriter(sliceMd5H2, SliceSize)
	writers := []io.Writer{fileMd5H, sliceMd5H, slicemd5H2Write}
	if tmpF != nil {
		writers = append(writers, tmpF)
	}
	written := int64(0)
	for i := 1; i <= count; i++ {
		if utils.IsCanceled(ctx) {
			return nil, ctx.Err()
		}
		if i == count {
			byteSize = lastBlockSize
		}
		n, err := utils.CopyWithBufferN(io.MultiWriter(writers...), stream, byteSize)
		written += n
		if err != nil && err != io.EOF {
			return nil, err
		}
		sliceMD5List = append(sliceMD5List, hex.EncodeToString(sliceMd5H.Sum(nil)))
		sliceMd5H.Reset()
	}
	if tmpF != nil {
		if written != streamSize {
			return nil, errs.NewErr(err, "CreateTempFile failed, incoming stream actual size= %d, expect = %d ", written, streamSize)
		}
		_, err = tmpF.Seek(0, io.SeekStart)
		if err != nil {
			return nil, errs.NewErr(err, "CreateTempFile failed, can't seek to 0 ")
		}
	}
	contentMd5 := hex.EncodeToString(fileMd5H.Sum(nil))
	sliceMd5 := hex.EncodeToString(sliceMd5H2.Sum(nil))
	blockListStr, _ := utils.Json.MarshalToString(sliceMD5List)

	// step.2 预上传
	params := map[string]string{
		"autoinit":    "1",
		"isdir":       "0",
		"rtype":       "1",
		"ctype":       "11",
		"path":        fmt.Sprintf("/%s", stream.GetName()),
		"size":        fmt.Sprint(streamSize),
		"slice-md5":   sliceMd5,
		"content-md5": contentMd5,
		"block_list":  blockListStr,
	}

	// 尝试获取之前的进度
	precreateResp, ok := base.GetUploadProgress[*PrecreateResp](d, strconv.FormatInt(d.Uk, 10), contentMd5)
	if !ok {
		_, err = d.Post(FILE_API_URL_V1+"/precreate", func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetFormData(params)
			r.SetQueryParam("bdstoken", d.bdstoken)
		}, &precreateResp)
		if err != nil {
			return nil, err
		}
	}

	switch precreateResp.ReturnType {
	case 1: //step.3 上传文件切片
		threadG, upCtx := errgroup.NewGroupWithContext(ctx, d.uploadThread,
			retry.Attempts(3),
			retry.Delay(time.Second),
			retry.DelayType(retry.BackOffDelay))
		sem := semaphore.NewWeighted(3)
		for i, partseq := range precreateResp.BlockList {
			if utils.IsCanceled(upCtx) {
				break
			}

			i, partseq, offset, byteSize := i, partseq, int64(partseq)*DEFAULT, DEFAULT
			if partseq+1 == count {
				byteSize = lastBlockSize
			}

			threadG.Go(func(ctx context.Context) error {
				if err = sem.Acquire(ctx, 1); err != nil {
					return err
				}
				defer sem.Release(1)
				uploadParams := map[string]string{
					"method":   "upload",
					"path":     params["path"],
					"partseq":  fmt.Sprint(partseq),
					"uploadid": precreateResp.UploadID,
					"app_id":   "16051585",
				}
				_, err = d.Post("https://c3.pcs.baidu.com/rest/2.0/pcs/superfile2", func(r *resty.Request) {
					r.SetContext(ctx)
					r.SetQueryParams(uploadParams)
					r.SetFileReader("file", stream.GetName(),
						driver.NewLimitedUploadStream(ctx, io.NewSectionReader(cache, offset, byteSize)))
				}, nil)
				if err != nil {
					return err
				}
				up(float64(threadG.Success()) * 100 / float64(len(precreateResp.BlockList)))
				precreateResp.BlockList[i] = -1
				return nil
			})
		}
		if err = threadG.Wait(); err != nil {
			if errors.Is(err, context.Canceled) {
				precreateResp.BlockList = utils.SliceFilter(precreateResp.BlockList, func(s int) bool { return s >= 0 })
				base.SaveUploadProgress(d, strconv.FormatInt(d.Uk, 10), contentMd5)
			}
			return nil, err
		}
		fallthrough
	case 2: //step.4 创建文件
		params["uploadid"] = precreateResp.UploadID
		_, err = d.Post(FILE_API_URL_V1+"/create", func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetFormData(params)
			r.SetQueryParam("bdstoken", d.bdstoken)
		}, &precreateResp)
		if err != nil {
			return nil, err
		}
		fallthrough
	case 3: //step.5 增加到相册
		rootfile := precreateResp.Data.toFile()
		if album, ok := dstDir.(*Album); ok {
			return d.AddAlbumFile(ctx, album, rootfile)
		}
		return rootfile, nil
	}
	return nil, errs.NotSupport
}

var _ driver.Driver = (*BaiduPhoto)(nil)
var _ driver.GetRooter = (*BaiduPhoto)(nil)
var _ driver.MkdirResult = (*BaiduPhoto)(nil)
var _ driver.CopyResult = (*BaiduPhoto)(nil)
var _ driver.MoveResult = (*BaiduPhoto)(nil)
var _ driver.Remove = (*BaiduPhoto)(nil)
var _ driver.PutResult = (*BaiduPhoto)(nil)
var _ driver.RenameResult = (*BaiduPhoto)(nil)
