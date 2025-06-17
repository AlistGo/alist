package fs

import (
	"context"
	log "github.com/sirupsen/logrus"
	"io"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
)

// the param named path of functions in this package is a mount path
// So, the purpose of this package is to convert mount path to actual path
// then pass the actual path to the op package

type ListArgs struct {
	Refresh bool
	NoLog   bool
}

func List(ctx context.Context, path string, args *ListArgs) ([]model.Obj, error) {
	res, err := list(ctx, path, args)
	if err != nil {
		if !args.NoLog {
			log.Errorf("failed list %s: %+v", path, err)
		}
		return nil, err
	}
	return res, nil
}

type GetArgs struct {
	NoLog bool
}

func Get(ctx context.Context, path string, args *GetArgs) (model.Obj, error) {
	res, err := get(ctx, path)
	if err != nil {
		if !args.NoLog {
			log.Warnf("failed get %s: %s", path, err)
		}
		return nil, err
	}
	return res, nil
}

func Link(ctx context.Context, path string, args model.LinkArgs) (*model.Link, model.Obj, error) {
	res, file, err := link(ctx, path, args)
	if err != nil {
		log.Errorf("failed link %s: %+v", path, err)
		return nil, nil, err
	}
	return res, file, nil
}

func MakeDir(ctx context.Context, path string, lazyCache ...bool) error {
	err := makeDir(ctx, path, lazyCache...)
	if err != nil {
		log.Errorf("failed make dir %s: %+v", path, err)
	}
	return err
}

func Move(ctx context.Context, srcPath, dstDirPath string, lazyCache ...bool) error {
	err := move(ctx, srcPath, dstDirPath, lazyCache...)
	if err != nil {
		log.Errorf("failed move %s to %s: %+v", srcPath, dstDirPath, err)
	}
	return err
}

func Copy(ctx context.Context, srcObjPath, dstDirPath string, lazyCache ...bool) (task.TaskExtensionInfo, error) {
	res, err := _copy(ctx, srcObjPath, dstDirPath, lazyCache...)
	if err != nil {
		log.Errorf("failed copy %s to %s: %+v", srcObjPath, dstDirPath, err)
	}
	return res, err
}

func Rename(ctx context.Context, srcPath, dstName string, lazyCache ...bool) error {
	err := rename(ctx, srcPath, dstName, lazyCache...)
	if err != nil {
		log.Errorf("failed rename %s to %s: %+v", srcPath, dstName, err)
	}
	return err
}

func Remove(ctx context.Context, path string) error {
	err := remove(ctx, path)
	if err != nil {
		log.Errorf("failed remove %s: %+v", path, err)
	}
	return err
}

func PutDirectly(ctx context.Context, dstDirPath string, file model.FileStreamer, lazyCache ...bool) error {
	err := putDirectly(ctx, dstDirPath, file, lazyCache...)
	if err != nil {
		log.Errorf("failed put %s: %+v", dstDirPath, err)
	}
	return err
}

func PutAsTask(ctx context.Context, dstDirPath string, file model.FileStreamer) (task.TaskExtensionInfo, error) {
	t, err := putAsTask(ctx, dstDirPath, file)
	if err != nil {
		log.Errorf("failed put %s: %+v", dstDirPath, err)
	}
	return t, err
}

func ArchiveMeta(ctx context.Context, path string, args model.ArchiveMetaArgs) (*model.ArchiveMetaProvider, error) {
	meta, err := archiveMeta(ctx, path, args)
	if err != nil {
		log.Errorf("failed get archive meta %s: %+v", path, err)
	}
	return meta, err
}

func ArchiveList(ctx context.Context, path string, args model.ArchiveListArgs) ([]model.Obj, error) {
	objs, err := archiveList(ctx, path, args)
	if err != nil {
		log.Errorf("failed list archive [%s]%s: %+v", path, args.InnerPath, err)
	}
	return objs, err
}

func ArchiveDecompress(ctx context.Context, srcObjPath, dstDirPath string, args model.ArchiveDecompressArgs, lazyCache ...bool) (task.TaskExtensionInfo, error) {
	t, err := archiveDecompress(ctx, srcObjPath, dstDirPath, args, lazyCache...)
	if err != nil {
		log.Errorf("failed decompress [%s]%s: %+v", srcObjPath, args.InnerPath, err)
	}
	return t, err
}

func ArchiveDriverExtract(ctx context.Context, path string, args model.ArchiveInnerArgs) (*model.Link, model.Obj, error) {
	l, obj, err := archiveDriverExtract(ctx, path, args)
	if err != nil {
		log.Errorf("failed extract [%s]%s: %+v", path, args.InnerPath, err)
	}
	return l, obj, err
}

func ArchiveInternalExtract(ctx context.Context, path string, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	l, obj, err := archiveInternalExtract(ctx, path, args)
	if err != nil {
		log.Errorf("failed extract [%s]%s: %+v", path, args.InnerPath, err)
	}
	return l, obj, err
}

type GetStoragesArgs struct {
}

func GetStorage(path string, args *GetStoragesArgs) (driver.Driver, error) {
	storageDriver, _, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return nil, err
	}
	return storageDriver, nil
}

func Other(ctx context.Context, args model.FsOtherArgs) (interface{}, error) {
	res, err := other(ctx, args)
	if err != nil {
		log.Errorf("failed remove %s: %+v", args.Path, err)
	}
	return res, err
}

func PutURL(ctx context.Context, path, dstName, urlStr string) error {
	storage, dstDirActualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	if storage.Config().NoUpload {
		return errors.WithStack(errs.UploadNotSupported)
	}
	_, ok := storage.(driver.PutURL)
	_, okResult := storage.(driver.PutURLResult)
	if !ok && !okResult {
		return errs.NotImplement
	}
	return op.PutURL(ctx, storage, dstDirActualPath, dstName, urlStr)
}
