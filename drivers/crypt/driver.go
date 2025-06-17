package crypt

import (
	"context"
	"fmt"
	"io"
	stdpath "path"
	"regexp"
	"strings"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/server/common"
	rcCrypt "github.com/rclone/rclone/backend/crypt"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/obscure"
	log "github.com/sirupsen/logrus"
)

type Crypt struct {
	model.Storage
	Addition
	cipher        *rcCrypt.Cipher
	remoteStorage driver.Driver
}

const obfuscatedPrefix = "___Obfuscated___"

func (d *Crypt) Config() driver.Config {
	return config
}

func (d *Crypt) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *Crypt) Init(ctx context.Context) error {
	//obfuscate credentials if it's updated or just created
	err := d.updateObfusParm(&d.Password)
	if err != nil {
		return fmt.Errorf("failed to obfuscate password: %w", err)
	}
	err = d.updateObfusParm(&d.Salt)
	if err != nil {
		return fmt.Errorf("failed to obfuscate salt: %w", err)
	}

	isCryptExt := regexp.MustCompile(`^[.][A-Za-z0-9-_]{2,}$`).MatchString
	if !isCryptExt(d.EncryptedSuffix) {
		return fmt.Errorf("EncryptedSuffix is Illegal")
	}
	d.FileNameEncoding = utils.GetNoneEmpty(d.FileNameEncoding, "base64")
	d.EncryptedSuffix = utils.GetNoneEmpty(d.EncryptedSuffix, ".bin")

	op.MustSaveDriverStorage(d)

	//need remote storage exist
	storage, err := fs.GetStorage(d.RemotePath, &fs.GetStoragesArgs{})
	if err != nil {
		return fmt.Errorf("can't find remote storage: %w", err)
	}
	d.remoteStorage = storage

	p, _ := strings.CutPrefix(d.Password, obfuscatedPrefix)
	p2, _ := strings.CutPrefix(d.Salt, obfuscatedPrefix)
	config := configmap.Simple{
		"password":                  p,
		"password2":                 p2,
		"filename_encryption":       d.FileNameEnc,
		"directory_name_encryption": d.DirNameEnc,
		"filename_encoding":         d.FileNameEncoding,
		"suffix":                    d.EncryptedSuffix,
		"pass_bad_blocks":           "",
	}
	c, err := rcCrypt.NewCipher(config)
	if err != nil {
		return fmt.Errorf("failed to create Cipher: %w", err)
	}
	d.cipher = c

	return nil
}

func (d *Crypt) updateObfusParm(str *string) error {
	temp := *str
	if !strings.HasPrefix(temp, obfuscatedPrefix) {
		temp, err := obscure.Obscure(temp)
		if err != nil {
			return err
		}
		temp = obfuscatedPrefix + temp
		*str = temp
	}
	return nil
}

func (d *Crypt) Drop(ctx context.Context) error {
	return nil
}

func (d *Crypt) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	path := dir.GetPath()
	//return d.list(ctx, d.RemotePath, path)
	//remoteFull

	objs, err := fs.List(ctx, d.getPathForRemote(path, true), &fs.ListArgs{NoLog: true})
	// the obj must implement the model.SetPath interface
	// return objs, err
	if err != nil {
		return nil, err
	}

	var result []model.Obj
	for _, obj := range objs {
		if obj.IsDir() {
			name, err := d.cipher.DecryptDirName(obj.GetName())
			if err != nil {
				//filter illegal files
				continue
			}
			if !d.ShowHidden && strings.HasPrefix(name, ".") {
				continue
			}
			objRes := model.Object{
				Name:     name,
				Size:     0,
				Modified: obj.ModTime(),
				IsFolder: obj.IsDir(),
				Ctime:    obj.CreateTime(),
				// discarding hash as it's encrypted
			}
			result = append(result, &objRes)
		} else {
			thumb, ok := model.GetThumb(obj)
			size, err := d.cipher.DecryptedSize(obj.GetSize())
			if err != nil {
				//filter illegal files
				continue
			}
			name, err := d.cipher.DecryptFileName(obj.GetName())
			if err != nil {
				//filter illegal files
				continue
			}
			if !d.ShowHidden && strings.HasPrefix(name, ".") {
				continue
			}
			objRes := model.Object{
				Name:     name,
				Size:     size,
				Modified: obj.ModTime(),
				IsFolder: obj.IsDir(),
				Ctime:    obj.CreateTime(),
				// discarding hash as it's encrypted
			}
			if d.Thumbnail && thumb == "" {
				thumbPath := stdpath.Join(args.ReqPath, ".thumbnails", name+".webp")
				thumb = fmt.Sprintf("%s/d%s?sign=%s",
					common.GetApiUrl(common.GetHttpReq(ctx)),
					utils.EncodePath(thumbPath, true),
					sign.Sign(thumbPath))
			}
			if !ok && !d.Thumbnail {
				result = append(result, &objRes)
			} else {
				objWithThumb := model.ObjThumb{
					Object: objRes,
					Thumbnail: model.Thumbnail{
						Thumbnail: thumb,
					},
				}
				result = append(result, &objWithThumb)
			}
		}
	}

	return result, nil
}

func (d *Crypt) Get(ctx context.Context, path string) (model.Obj, error) {
	if utils.PathEqual(path, "/") {
		return &model.Object{
			Name:     "Root",
			IsFolder: true,
			Path:     "/",
		}, nil
	}
	remoteFullPath := ""
	var remoteObj model.Obj
	var err, err2 error
	firstTryIsFolder, secondTry := guessPath(path)
	remoteFullPath = d.getPathForRemote(path, firstTryIsFolder)
	remoteObj, err = fs.Get(ctx, remoteFullPath, &fs.GetArgs{NoLog: true})
	if err != nil {
		if errs.IsObjectNotFound(err) && secondTry {
			//try the opposite
			remoteFullPath = d.getPathForRemote(path, !firstTryIsFolder)
			remoteObj, err2 = fs.Get(ctx, remoteFullPath, &fs.GetArgs{NoLog: true})
			if err2 != nil {
				return nil, err2
			}
		} else {
			return nil, err
		}
	}
	var size int64 = 0
	name := ""
	if !remoteObj.IsDir() {
		size, err = d.cipher.DecryptedSize(remoteObj.GetSize())
		if err != nil {
			log.Warnf("DecryptedSize failed for %s ,will use original size, err:%s", path, err)
			size = remoteObj.GetSize()
		}
		name, err = d.cipher.DecryptFileName(remoteObj.GetName())
		if err != nil {
			log.Warnf("DecryptFileName failed for %s ,will use original name, err:%s", path, err)
			name = remoteObj.GetName()
		}
	} else {
		name, err = d.cipher.DecryptDirName(remoteObj.GetName())
		if err != nil {
			log.Warnf("DecryptDirName failed for %s ,will use original name, err:%s", path, err)
			name = remoteObj.GetName()
		}
	}
	obj := &model.Object{
		Path:     path,
		Name:     name,
		Size:     size,
		Modified: remoteObj.ModTime(),
		IsFolder: remoteObj.IsDir(),
	}
	return obj, nil
	//return nil, errs.ObjectNotFound
}

func (d *Crypt) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	dstDirActualPath, err := d.getActualPathForRemote(file.GetPath(), false)
	if err != nil {
		return nil, fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	remoteLink, remoteFile, err := op.Link(ctx, d.remoteStorage, dstDirActualPath, args)
	if err != nil {
		return nil, err
	}

	if remoteLink.RangeReadCloser == nil && remoteLink.MFile == nil && len(remoteLink.URL) == 0 {
		return nil, fmt.Errorf("the remote storage driver need to be enhanced to support encrytion")
	}
	remoteFileSize := remoteFile.GetSize()
	remoteClosers := utils.EmptyClosers()
	rangeReaderFunc := func(ctx context.Context, underlyingOffset, underlyingLength int64) (io.ReadCloser, error) {
		length := underlyingLength
		if underlyingLength >= 0 && underlyingOffset+underlyingLength >= remoteFileSize {
			length = -1
		}
		rrc := remoteLink.RangeReadCloser
		if len(remoteLink.URL) > 0 {
			var converted, err = stream.GetRangeReadCloserFromLink(remoteFileSize, remoteLink)
			if err != nil {
				return nil, err
			}
			rrc = converted
		}
		if rrc != nil {
			remoteReader, err := rrc.RangeRead(ctx, http_range.Range{Start: underlyingOffset, Length: length})
			remoteClosers.AddClosers(rrc.GetClosers())
			if err != nil {
				return nil, err
			}
			return remoteReader, nil
		}
		if remoteLink.MFile != nil {
			_, err := remoteLink.MFile.Seek(underlyingOffset, io.SeekStart)
			if err != nil {
				return nil, err
			}
			//keep reuse same MFile and close at last.
			remoteClosers.Add(remoteLink.MFile)
			return io.NopCloser(remoteLink.MFile), nil
		}

		return nil, errs.NotSupport

	}
	resultRangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
		readSeeker, err := d.cipher.DecryptDataSeek(ctx, rangeReaderFunc, httpRange.Start, httpRange.Length)
		if err != nil {
			return nil, err
		}
		return readSeeker, nil
	}

	resultRangeReadCloser := &model.RangeReadCloser{RangeReader: resultRangeReader, Closers: remoteClosers}
	resultLink := &model.Link{
		RangeReadCloser: resultRangeReadCloser,
		Expiration:      remoteLink.Expiration,
	}

	return resultLink, nil

}

func (d *Crypt) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	dstDirActualPath, err := d.getActualPathForRemote(parentDir.GetPath(), true)
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	dir := d.cipher.EncryptDirName(dirName)
	return op.MakeDir(ctx, d.remoteStorage, stdpath.Join(dstDirActualPath, dir))
}

func (d *Crypt) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcRemoteActualPath, err := d.getActualPathForRemote(srcObj.GetPath(), srcObj.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	dstRemoteActualPath, err := d.getActualPathForRemote(dstDir.GetPath(), dstDir.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	return op.Move(ctx, d.remoteStorage, srcRemoteActualPath, dstRemoteActualPath)
}

func (d *Crypt) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	remoteActualPath, err := d.getActualPathForRemote(srcObj.GetPath(), srcObj.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	var newEncryptedName string
	if srcObj.IsDir() {
		newEncryptedName = d.cipher.EncryptDirName(newName)
	} else {
		newEncryptedName = d.cipher.EncryptFileName(newName)
	}
	return op.Rename(ctx, d.remoteStorage, remoteActualPath, newEncryptedName)
}

func (d *Crypt) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	srcRemoteActualPath, err := d.getActualPathForRemote(srcObj.GetPath(), srcObj.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	dstRemoteActualPath, err := d.getActualPathForRemote(dstDir.GetPath(), dstDir.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	return op.Copy(ctx, d.remoteStorage, srcRemoteActualPath, dstRemoteActualPath)

}

func (d *Crypt) Remove(ctx context.Context, obj model.Obj) error {
	remoteActualPath, err := d.getActualPathForRemote(obj.GetPath(), obj.IsDir())
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}
	return op.Remove(ctx, d.remoteStorage, remoteActualPath)
}

func (d *Crypt) Put(ctx context.Context, dstDir model.Obj, streamer model.FileStreamer, up driver.UpdateProgress) error {
	dstDirActualPath, err := d.getActualPathForRemote(dstDir.GetPath(), true)
	if err != nil {
		return fmt.Errorf("failed to convert path to remote path: %w", err)
	}

	// Encrypt the data into wrappedIn
	wrappedIn, err := d.cipher.EncryptData(streamer)
	if err != nil {
		return fmt.Errorf("failed to EncryptData: %w", err)
	}

	// doesn't support seekableStream, since rapid-upload is not working for encrypted data
	streamOut := &stream.FileStream{
		Obj: &model.Object{
			ID:       streamer.GetID(),
			Path:     streamer.GetPath(),
			Name:     d.cipher.EncryptFileName(streamer.GetName()),
			Size:     d.cipher.EncryptedSize(streamer.GetSize()),
			Modified: streamer.ModTime(),
			IsFolder: streamer.IsDir(),
		},
		Reader:            wrappedIn,
		Mimetype:          "application/octet-stream",
		WebPutAsTask:      streamer.NeedStore(),
		ForceStreamUpload: true,
		Exist:             streamer.GetExist(),
	}
	err = op.Put(ctx, d.remoteStorage, dstDirActualPath, streamOut, up, false)
	if err != nil {
		return err
	}
	return nil
}

//func (d *Safe) Other(ctx context.Context, args model.OtherArgs) (interface{}, error) {
//	return nil, errs.NotSupport
//}

var _ driver.Driver = (*Crypt)(nil)
