package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	stdpath "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/alist-org/times"
	cp "github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"
	_ "golang.org/x/image/webp"
)

type Local struct {
	model.Storage
	Addition
	mkdirPerm int32

	// zero means no limit
	thumbConcurrency int
	thumbTokenBucket TokenBucket

	// video thumb position
	videoThumbPos             float64
	videoThumbPosIsPercentage bool
}

func (d *Local) Config() driver.Config {
	return config
}

func (d *Local) Init(ctx context.Context) error {
	if d.MkdirPerm == "" {
		d.mkdirPerm = 0777
	} else {
		v, err := strconv.ParseUint(d.MkdirPerm, 8, 32)
		if err != nil {
			return err
		}
		d.mkdirPerm = int32(v)
	}
	if !utils.Exists(d.GetRootPath()) {
		return fmt.Errorf("root folder %s not exists", d.GetRootPath())
	}
	if !filepath.IsAbs(d.GetRootPath()) {
		abs, err := filepath.Abs(d.GetRootPath())
		if err != nil {
			return err
		}
		d.Addition.RootFolderPath = abs
	}
	if d.ThumbCacheFolder != "" && !utils.Exists(d.ThumbCacheFolder) {
		err := os.MkdirAll(d.ThumbCacheFolder, os.FileMode(d.mkdirPerm))
		if err != nil {
			return err
		}
	}
	if d.ThumbConcurrency != "" {
		v, err := strconv.ParseUint(d.ThumbConcurrency, 10, 32)
		if err != nil {
			return err
		}
		d.thumbConcurrency = int(v)
	}
	if d.thumbConcurrency == 0 {
		d.thumbTokenBucket = NewNopTokenBucket()
	} else {
		d.thumbTokenBucket = NewStaticTokenBucketWithMigration(d.thumbTokenBucket, d.thumbConcurrency)
	}
	// Check the VideoThumbPos value
	if d.VideoThumbPos == "" {
		d.VideoThumbPos = "20%"
	}
	if strings.HasSuffix(d.VideoThumbPos, "%") {
		percentage := strings.TrimSuffix(d.VideoThumbPos, "%")
		val, err := strconv.ParseFloat(percentage, 64)
		if err != nil {
			return fmt.Errorf("invalid video_thumb_pos value: %s, err: %s", d.VideoThumbPos, err)
		}
		if val < 0 || val > 100 {
			return fmt.Errorf("invalid video_thumb_pos value: %s, the precentage must be a number between 0 and 100", d.VideoThumbPos)
		}
		d.videoThumbPosIsPercentage = true
		d.videoThumbPos = val / 100
	} else {
		val, err := strconv.ParseFloat(d.VideoThumbPos, 64)
		if err != nil {
			return fmt.Errorf("invalid video_thumb_pos value: %s, err: %s", d.VideoThumbPos, err)
		}
		if val < 0 {
			return fmt.Errorf("invalid video_thumb_pos value: %s, the time must be a positive number", d.VideoThumbPos)
		}
		d.videoThumbPosIsPercentage = false
		d.videoThumbPos = val
	}
	return nil
}

func (d *Local) Drop(ctx context.Context) error {
	return nil
}

func (d *Local) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Local) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	fullPath := dir.GetPath()
	rawFiles, err := readDir(fullPath)
	if err != nil {
		return nil, err
	}
	var files []model.Obj
	for _, f := range rawFiles {
		if !d.ShowHidden && strings.HasPrefix(f.Name(), ".") {
			continue
		}
		file := d.FileInfoToObj(ctx, f, args.ReqPath, fullPath)
		files = append(files, file)
	}
	return files, nil
}
func (d *Local) FileInfoToObj(ctx context.Context, f fs.FileInfo, reqPath string, fullPath string) model.Obj {
	thumb := ""
	if d.Thumbnail {
		typeName := utils.GetFileType(f.Name())
		if typeName == conf.IMAGE || typeName == conf.VIDEO {
			thumb = common.GetApiUrl(common.GetHttpReq(ctx)) + stdpath.Join("/d", reqPath, f.Name())
			thumb = utils.EncodePath(thumb, true)
			thumb += "?type=thumb&sign=" + sign.Sign(stdpath.Join(reqPath, f.Name()))
		}
	}
	isFolder := f.IsDir() || isSymlinkDir(f, fullPath)
	var size int64
	if !isFolder {
		size = f.Size()
	}
	var ctime time.Time
	t, err := times.Stat(stdpath.Join(fullPath, f.Name()))
	if err == nil {
		if t.HasBirthTime() {
			ctime = t.BirthTime()
		}
	}

	file := model.ObjThumb{
		Object: model.Object{
			Path:     filepath.Join(fullPath, f.Name()),
			Name:     f.Name(),
			Modified: f.ModTime(),
			Size:     size,
			IsFolder: isFolder,
			Ctime:    ctime,
		},
		Thumbnail: model.Thumbnail{
			Thumbnail: thumb,
		},
	}
	return &file
}
func (d *Local) GetMeta(ctx context.Context, path string) (model.Obj, error) {
	f, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	file := d.FileInfoToObj(ctx, f, path, path)
	//h := "123123"
	//if s, ok := f.(model.SetHash); ok && file.GetHash() == ("","")  {
	//	s.SetHash(h,"SHA1")
	//}
	return file, nil

}

func (d *Local) Get(ctx context.Context, path string) (model.Obj, error) {
	path = filepath.Join(d.GetRootPath(), path)
	f, err := os.Stat(path)
	if err != nil {
		if strings.Contains(err.Error(), "cannot find the file") {
			return nil, errs.ObjectNotFound
		}
		return nil, err
	}
	isFolder := f.IsDir() || isSymlinkDir(f, path)
	size := f.Size()
	if isFolder {
		size = 0
	}
	var ctime time.Time
	t, err := times.Stat(path)
	if err == nil {
		if t.HasBirthTime() {
			ctime = t.BirthTime()
		}
	}
	file := model.Object{
		Path:     path,
		Name:     f.Name(),
		Modified: f.ModTime(),
		Ctime:    ctime,
		Size:     size,
		IsFolder: isFolder,
	}
	return &file, nil
}

func (d *Local) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	fullPath := file.GetPath()
	var link model.Link
	if args.Type == "thumb" && utils.Ext(file.GetName()) != "svg" {
		var buf *bytes.Buffer
		var thumbPath *string
		err := d.thumbTokenBucket.Do(ctx, func() error {
			var err error
			buf, thumbPath, err = d.getThumb(file)
			return err
		})
		if err != nil {
			return nil, err
		}
		link.Header = http.Header{
			"Content-Type": []string{"image/png"},
		}
		if thumbPath != nil {
			open, err := os.Open(*thumbPath)
			if err != nil {
				return nil, err
			}
			link.MFile = open
		} else {
			link.MFile = model.NewNopMFile(bytes.NewReader(buf.Bytes()))
			//link.Header.Set("Content-Length", strconv.Itoa(buf.Len()))
		}
	} else {
		open, err := os.Open(fullPath)
		if err != nil {
			return nil, err
		}
		link.MFile = open
	}
	return &link, nil
}

func (d *Local) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	fullPath := filepath.Join(parentDir.GetPath(), dirName)
	err := os.MkdirAll(fullPath, os.FileMode(d.mkdirPerm))
	if err != nil {
		return err
	}
	return nil
}

func (d *Local) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	if utils.IsSubPath(srcPath, dstPath) {
		return fmt.Errorf("the destination folder is a subfolder of the source folder")
	}
	if err := os.Rename(srcPath, dstPath); err != nil && strings.Contains(err.Error(), "invalid cross-device link") {
		// Handle cross-device file move in local driver
		if err = d.Copy(ctx, srcObj, dstDir); err != nil {
			return err
		} else {
			// Directly remove file without check recycle bin if successfully copied
			if srcObj.IsDir() {
				err = os.RemoveAll(srcObj.GetPath())
			} else {
				err = os.Remove(srcObj.GetPath())
			}
			return err
		}
	} else {
		return err
	}
}

func (d *Local) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(filepath.Dir(srcPath), newName)
	err := os.Rename(srcPath, dstPath)
	if err != nil {
		return err
	}
	return nil
}

func (d *Local) Copy(_ context.Context, srcObj, dstDir model.Obj) error {
	srcPath := srcObj.GetPath()
	dstPath := filepath.Join(dstDir.GetPath(), srcObj.GetName())
	if utils.IsSubPath(srcPath, dstPath) {
		return fmt.Errorf("the destination folder is a subfolder of the source folder")
	}
	// Copy using otiai10/copy to perform more secure & efficient copy
	return cp.Copy(srcPath, dstPath, cp.Options{
		Sync:          true, // Sync file to disk after copy, may have performance penalty in filesystem such as ZFS
		PreserveTimes: true,
		PreserveOwner: true,
	})
}

func (d *Local) Remove(ctx context.Context, obj model.Obj) error {
	var err error
	if utils.SliceContains([]string{"", "delete permanently"}, d.RecycleBinPath) {
		if obj.IsDir() {
			err = os.RemoveAll(obj.GetPath())
		} else {
			err = os.Remove(obj.GetPath())
		}
	} else {
		dstPath := filepath.Join(d.RecycleBinPath, obj.GetName())
		if utils.Exists(dstPath) {
			dstPath = filepath.Join(d.RecycleBinPath, obj.GetName()+"_"+time.Now().Format("20060102150405"))
		}
		err = os.Rename(obj.GetPath(), dstPath)
	}
	if err != nil {
		return err
	}
	return nil
}

func (d *Local) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) error {
	fullPath := filepath.Join(dstDir.GetPath(), stream.GetName())
	out, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
		if errors.Is(err, context.Canceled) {
			_ = os.Remove(fullPath)
		}
	}()
	err = utils.CopyWithCtx(ctx, out, stream, stream.GetSize(), up)
	if err != nil {
		return err
	}
	err = os.Chtimes(fullPath, stream.ModTime(), stream.ModTime())
	if err != nil {
		log.Errorf("[local] failed to change time of %s: %s", fullPath, err)
	}
	return nil
}

var _ driver.Driver = (*Local)(nil)
