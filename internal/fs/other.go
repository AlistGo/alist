package fs

import (
	"context"
	"encoding/json"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/drivers/s3"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/task"
	"github.com/pkg/errors"
)

func makeDir(ctx context.Context, path string, lazyCache ...bool) error {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	return op.MakeDir(ctx, storage, actualPath, lazyCache...)
}

func move(ctx context.Context, srcPath, dstDirPath string, lazyCache ...bool) error {
	srcStorage, srcActualPath, err := op.GetStorageAndActualPath(srcPath)
	if err != nil {
		return errors.WithMessage(err, "failed get src storage")
	}
	dstStorage, dstDirActualPath, err := op.GetStorageAndActualPath(dstDirPath)
	if err != nil {
		return errors.WithMessage(err, "failed get dst storage")
	}
	if srcStorage.GetStorage() != dstStorage.GetStorage() {
		return errors.WithStack(errs.MoveBetweenTwoStorages)
	}
	return op.Move(ctx, srcStorage, srcActualPath, dstDirActualPath, lazyCache...)
}

func rename(ctx context.Context, srcPath, dstName string, lazyCache ...bool) error {
	storage, srcActualPath, err := op.GetStorageAndActualPath(srcPath)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	return op.Rename(ctx, storage, srcActualPath, dstName, lazyCache...)
}

func remove(ctx context.Context, path string) error {
	storage, actualPath, err := op.GetStorageAndActualPath(path)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	return op.Remove(ctx, storage, actualPath)
}

func other(ctx context.Context, args model.FsOtherArgs) (interface{}, error) {
	storage, actualPath, err := op.GetStorageAndActualPath(args.Path)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get storage")
	}
	originalPath := args.Path

	if _, ok := storage.(*s3.S3); ok {
		method := strings.ToLower(strings.TrimSpace(args.Method))
		if method == s3.OtherMethodArchive || method == s3.OtherMethodThaw {
			if S3TransitionTaskManager == nil {
				return nil, errors.New("s3 transition task manager is not initialized")
			}
			var payload json.RawMessage
			if args.Data != nil {
				raw, err := json.Marshal(args.Data)
				if err != nil {
					return nil, errors.WithMessage(err, "failed to encode request payload")
				}
				payload = raw
			}
			taskCreator, _ := ctx.Value("user").(*model.User)
			tsk := &S3TransitionTask{
				TaskExtension:    task.TaskExtension{Creator: taskCreator},
				status:           "queued",
				StorageMountPath: storage.GetStorage().MountPath,
				ObjectPath:       actualPath,
				DisplayPath:      originalPath,
				ObjectName:       stdpath.Base(actualPath),
				Transition:       method,
				Payload:          payload,
			}
			S3TransitionTaskManager.Add(tsk)
			return map[string]string{"task_id": tsk.GetID()}, nil
		}
	}

	args.Path = actualPath
	return op.Other(ctx, storage, args)
}
