package baidu_netdisk

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	stdpath "path"
	"strconv"
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
	log "github.com/sirupsen/logrus"
)

type BaiduNetdisk struct {
	model.Storage
	Addition

	uploadThread int
	vipType      int // 会员类型，0普通用户(4G/4M)、1普通会员(10G/16M)、2超级会员(20G/32M)
}

func (d *BaiduNetdisk) Config() driver.Config {
	return config
}

func (d *BaiduNetdisk) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *BaiduNetdisk) Init(ctx context.Context) error {
	d.uploadThread, _ = strconv.Atoi(d.UploadThread)
	if d.uploadThread < 1 || d.uploadThread > 32 {
		d.uploadThread, d.UploadThread = 3, "3"
	}

	if _, err := url.Parse(d.UploadAPI); d.UploadAPI == "" || err != nil {
		d.UploadAPI = "https://d.pcs.baidu.com"
	}

	res, err := d.get("/xpan/nas", map[string]string{
		"method": "uinfo",
	}, nil)
	log.Debugf("[baidu] get uinfo: %s", string(res))
	if err != nil {
		return err
	}
	d.vipType = utils.Json.Get(res, "vip_type").ToInt()
	return nil
}

func (d *BaiduNetdisk) Drop(ctx context.Context) error {
	return nil
}

func (d *BaiduNetdisk) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	files, err := d.getFiles(dir.GetPath())
	if err != nil {
		return nil, err
	}
	return utils.SliceConvert(files, func(src File) (model.Obj, error) {
		return fileToObj(src), nil
	})
}

func (d *BaiduNetdisk) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if d.DownloadAPI == "crack" {
		return d.linkCrack(file, args)
	} else if d.DownloadAPI == "crack_video" {
		return d.linkCrackVideo(file, args)
	}
	return d.linkOfficial(file, args)
}

func (d *BaiduNetdisk) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	var newDir File
	_, err := d.create(stdpath.Join(parentDir.GetPath(), dirName), 0, 1, "", "", &newDir, 0, 0)
	if err != nil {
		return nil, err
	}
	return fileToObj(newDir), nil
}

func (d *BaiduNetdisk) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	data := []base.Json{
		{
			"path":    srcObj.GetPath(),
			"dest":    dstDir.GetPath(),
			"newname": srcObj.GetName(),
		},
	}
	_, err := d.manage("move", data)
	if err != nil {
		return nil, err
	}
	if srcObj, ok := srcObj.(*model.ObjThumb); ok {
		srcObj.SetPath(stdpath.Join(dstDir.GetPath(), srcObj.GetName()))
		srcObj.Modified = time.Now()
		return srcObj, nil
	}
	return nil, nil
}

func (d *BaiduNetdisk) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	data := []base.Json{
		{
			"path":    srcObj.GetPath(),
			"newname": newName,
		},
	}
	_, err := d.manage("rename", data)
	if err != nil {
		return nil, err
	}

	if srcObj, ok := srcObj.(*model.ObjThumb); ok {
		srcObj.SetPath(stdpath.Join(stdpath.Dir(srcObj.GetPath()), newName))
		srcObj.Name = newName
		srcObj.Modified = time.Now()
		return srcObj, nil
	}
	return nil, nil
}

func (d *BaiduNetdisk) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	data := []base.Json{
		{
			"path":    srcObj.GetPath(),
			"dest":    dstDir.GetPath(),
			"newname": srcObj.GetName(),
		},
	}
	_, err := d.manage("copy", data)
	return err
}

func (d *BaiduNetdisk) Remove(ctx context.Context, obj model.Obj) error {
	data := []string{obj.GetPath()}
	_, err := d.manage("delete", data)
	return err
}

func (d *BaiduNetdisk) PutRapid(ctx context.Context, dstDir model.Obj, stream model.FileStreamer) (model.Obj, error) {
	contentMd5 := stream.GetHash().GetHash(utils.MD5)
	if len(contentMd5) < utils.MD5.Width {
		return nil, errors.New("invalid hash")
	}

	streamSize := stream.GetSize()
	path := stdpath.Join(dstDir.GetPath(), stream.GetName())
	mtime := stream.ModTime().Unix()
	ctime := stream.CreateTime().Unix()
	blockList, _ := utils.Json.MarshalToString([]string{contentMd5})

	var newFile File
	_, err := d.create(path, streamSize, 0, "", blockList, &newFile, mtime, ctime)
	if err != nil {
		return nil, err
	}
	// 修复时间，具体原因见 Put 方法注释的 **注意**
	newFile.Ctime = stream.CreateTime().Unix()
	newFile.Mtime = stream.ModTime().Unix()
	return fileToObj(newFile), nil
}

// Put
//
// **注意**: 截至 2024/04/20 百度云盘 api 接口返回的时间永远是当前时间，而不是文件时间。
// 而实际上云盘存储的时间是文件时间，所以此处需要覆盖时间，保证缓存与云盘的数据一致
func (d *BaiduNetdisk) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	// rapid upload
	if newObj, err := d.PutRapid(ctx, dstDir, stream); err == nil {
		return newObj, nil
	}

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

	streamSize := stream.GetSize()
	sliceSize := d.getSliceSize(streamSize)
	count := int(streamSize / sliceSize)
	lastBlockSize := streamSize % sliceSize
	if lastBlockSize > 0 {
		count++
	} else {
		lastBlockSize = sliceSize
	}

	//cal md5 for first 256k data
	const SliceSize int64 = 256 * utils.KB
	// cal md5
	blockList := make([]string, 0, count)
	byteSize := sliceSize
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
		blockList = append(blockList, hex.EncodeToString(sliceMd5H.Sum(nil)))
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
	blockListStr, _ := utils.Json.MarshalToString(blockList)
	path := stdpath.Join(dstDir.GetPath(), stream.GetName())
	mtime := stream.ModTime().Unix()
	ctime := stream.CreateTime().Unix()

	// step.1 预上传
	// 尝试获取之前的进度
	precreateResp, ok := base.GetUploadProgress[*PrecreateResp](d, d.AccessToken, contentMd5)
	if !ok {
		params := map[string]string{
			"method": "precreate",
		}
		form := map[string]string{
			"path":        path,
			"size":        strconv.FormatInt(streamSize, 10),
			"isdir":       "0",
			"autoinit":    "1",
			"rtype":       "3",
			"block_list":  blockListStr,
			"content-md5": contentMd5,
			"slice-md5":   sliceMd5,
		}
		joinTime(form, ctime, mtime)

		log.Debugf("[baidu_netdisk] precreate data: %s", form)
		_, err = d.postForm("/xpan/file", params, form, &precreateResp)
		if err != nil {
			return nil, err
		}
		log.Debugf("%+v", precreateResp)
		if precreateResp.ReturnType == 2 {
			//rapid upload, since got md5 match from baidu server
			// 修复时间，具体原因见 Put 方法注释的 **注意**
			precreateResp.File.Ctime = ctime
			precreateResp.File.Mtime = mtime
			return fileToObj(precreateResp.File), nil
		}
	}
	// step.2 上传分片
	threadG, upCtx := errgroup.NewGroupWithContext(ctx, d.uploadThread,
		retry.Attempts(1),
		retry.Delay(time.Second),
		retry.DelayType(retry.BackOffDelay))
	sem := semaphore.NewWeighted(3)
	for i, partseq := range precreateResp.BlockList {
		if utils.IsCanceled(upCtx) {
			break
		}

		i, partseq, offset, byteSize := i, partseq, int64(partseq)*sliceSize, sliceSize
		if partseq+1 == count {
			byteSize = lastBlockSize
		}
		threadG.Go(func(ctx context.Context) error {
			if err = sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)
			params := map[string]string{
				"method":       "upload",
				"access_token": d.AccessToken,
				"type":         "tmpfile",
				"path":         path,
				"uploadid":     precreateResp.Uploadid,
				"partseq":      strconv.Itoa(partseq),
			}
			err := d.uploadSlice(ctx, params, stream.GetName(),
				driver.NewLimitedUploadStream(ctx, io.NewSectionReader(cache, offset, byteSize)))
			if err != nil {
				return err
			}
			up(float64(threadG.Success()) * 100 / float64(len(precreateResp.BlockList)))
			precreateResp.BlockList[i] = -1
			return nil
		})
	}
	if err = threadG.Wait(); err != nil {
		// 如果属于用户主动取消，则保存上传进度
		if errors.Is(err, context.Canceled) {
			precreateResp.BlockList = utils.SliceFilter(precreateResp.BlockList, func(s int) bool { return s >= 0 })
			base.SaveUploadProgress(d, precreateResp, d.AccessToken, contentMd5)
		}
		return nil, err
	}

	// step.3 创建文件
	var newFile File
	_, err = d.create(path, streamSize, 0, precreateResp.Uploadid, blockListStr, &newFile, mtime, ctime)
	if err != nil {
		return nil, err
	}
	// 修复时间，具体原因见 Put 方法注释的 **注意**
	newFile.Ctime = ctime
	newFile.Mtime = mtime
	return fileToObj(newFile), nil
}

func (d *BaiduNetdisk) uploadSlice(ctx context.Context, params map[string]string, fileName string, file io.Reader) error {
	res, err := base.RestyClient.R().
		SetContext(ctx).
		SetQueryParams(params).
		SetFileReader("file", fileName, file).
		Post(d.UploadAPI + "/rest/2.0/pcs/superfile2")
	if err != nil {
		return err
	}
	log.Debugln(res.RawResponse.Status + res.String())
	errCode := utils.Json.Get(res.Body(), "error_code").ToInt()
	errNo := utils.Json.Get(res.Body(), "errno").ToInt()
	if errCode != 0 || errNo != 0 {
		return errs.NewErr(errs.StreamIncomplete, "error in uploading to baidu, will retry. response=%s", res.String())
	}
	return nil
}

var _ driver.Driver = (*BaiduNetdisk)(nil)
