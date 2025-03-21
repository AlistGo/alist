package driver

import (
	"context"

	"github.com/alist-org/alist/v3/internal/model"
)

type Driver interface {
	Meta
	Reader
	//Writer
	//Other
}

type Meta interface {
	Config() Config
	// GetStorage just get raw storage, no need to implement, because model.Storage have implemented
	GetStorage() *model.Storage
	SetStorage(model.Storage)
	// GetAddition Additional is used for unmarshal of JSON, so need return pointer
	GetAddition() Additional
	// Init If already initialized, drop first
	Init(ctx context.Context) error
	Drop(ctx context.Context) error
}

type Other interface {
	Other(ctx context.Context, args model.OtherArgs) (interface{}, error)
}

type Reader interface {
	// List files in the path
	// if identify files by path, need to set ID with path,like path.Join(dir.GetID(), obj.GetName())
	// if identify files by id, need to set ID with corresponding id
	List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error)
	// Link get url/filepath/reader of file
	Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error)
}

type GetRooter interface {
	GetRoot(ctx context.Context) (model.Obj, error)
}

type Getter interface {
	// Get file by path, the path haven't been joined with root path
	Get(ctx context.Context, path string) (model.Obj, error)
}

//type Writer interface {
//	Mkdir
//	Move
//	Rename
//	Copy
//	Remove
//	Put
//}

type Mkdir interface {
	MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error
}

type Move interface {
	Move(ctx context.Context, srcObj, dstDir model.Obj) error
}

type Rename interface {
	Rename(ctx context.Context, srcObj model.Obj, newName string) error
}

type Copy interface {
	Copy(ctx context.Context, srcObj, dstDir model.Obj) error
}

type Remove interface {
	Remove(ctx context.Context, obj model.Obj) error
}

type Put interface {
	// Put a file (provided as a FileStreamer) into the driver
	// Besides the most basic upload functionality, the following features also need to be implemented:
	// 1. Canceling (when `<-ctx.Done()` returns), which can be supported by the following methods:
	//   (1) Use request methods that carry context, such as the following:
	//      a. http.NewRequestWithContext
	//      b. resty.Request.SetContext
	//      c. s3manager.Uploader.UploadWithContext
	//      d. utils.CopyWithCtx
	//   (2) Use a `driver.ReaderWithCtx` or `driver.NewLimitedUploadStream`
	//   (3) Use `utils.IsCanceled` to check if the upload has been canceled during the upload process,
	//       this is typically applicable to chunked uploads.
	// 2. Submit upload progress (via `up`) in real-time. There are three recommended ways as follows:
	//   (1) Use `utils.CopyWithCtx`
	//   (2) Use `driver.ReaderUpdatingProgress`
	//   (3) Use `driver.Progress` with `io.TeeReader`
	// 3. Slow down upload speed (via `stream.ServerUploadLimit`). It requires you to wrap the read stream
	//    in a `driver.RateLimitReader` or a `driver.RateLimitFile` after calculating the file's hash and
	//    before uploading the file or file chunks. Or you can directly call `driver.ServerUploadLimitWaitN`
	//    if your file chunks are sufficiently small (less than about 50KB).
	// NOTE that the network speed may be significantly slower than the stream's read speed. Therefore, if
	// you use a `errgroup.Group` to upload each chunk in parallel, you should consider using a recursive
	// mutex like `semaphore.Weighted` to limit the maximum number of upload threads, preventing excessive
	// memory usage caused by buffering too many file chunks awaiting upload.
	Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up UpdateProgress) error
}

type PutURL interface {
	// PutURL directly put a URL into the storage
	// Applicable to index-based drivers like URL-Tree or drivers that support uploading files as URLs
	// Called when using SimpleHttp for offline downloading, skipping creating a download task
	PutURL(ctx context.Context, dstDir model.Obj, name, url string) error
}

//type WriteResult interface {
//	MkdirResult
//	MoveResult
//	RenameResult
//	CopyResult
//	PutResult
//	Remove
//}

type MkdirResult interface {
	MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error)
}

type MoveResult interface {
	Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error)
}

type RenameResult interface {
	Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error)
}

type CopyResult interface {
	Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error)
}

type PutResult interface {
	// Put a file (provided as a FileStreamer) into the driver and return the put obj
	// Besides the most basic upload functionality, the following features also need to be implemented:
	// 1. Canceling (when `<-ctx.Done()` returns), which can be supported by the following methods:
	//   (1) Use request methods that carry context, such as the following:
	//      a. http.NewRequestWithContext
	//      b. resty.Request.SetContext
	//      c. s3manager.Uploader.UploadWithContext
	//      d. utils.CopyWithCtx
	//   (2) Use a `driver.ReaderWithCtx` or `driver.NewLimitedUploadStream`
	//   (3) Use `utils.IsCanceled` to check if the upload has been canceled during the upload process,
	//       this is typically applicable to chunked uploads.
	// 2. Submit upload progress (via `up`) in real-time. There are three recommended ways as follows:
	//   (1) Use `utils.CopyWithCtx`
	//   (2) Use `driver.ReaderUpdatingProgress`
	//   (3) Use `driver.Progress` with `io.TeeReader`
	// 3. Slow down upload speed (via `stream.ServerUploadLimit`). It requires you to wrap the read stream
	//    in a `driver.RateLimitReader` or a `driver.RateLimitFile` after calculating the file's hash and
	//    before uploading the file or file chunks. Or you can directly call `driver.ServerUploadLimitWaitN`
	//    if your file chunks are sufficiently small (less than about 50KB).
	// NOTE that the network speed may be significantly slower than the stream's read speed. Therefore, if
	// you use a `errgroup.Group` to upload each chunk in parallel, you should consider using a recursive
	// mutex like `semaphore.Weighted` to limit the maximum number of upload threads, preventing excessive
	// memory usage caused by buffering too many file chunks awaiting upload.
	Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up UpdateProgress) (model.Obj, error)
}

type PutURLResult interface {
	// PutURL directly put a URL into the storage
	// Applicable to index-based drivers like URL-Tree or drivers that support uploading files as URLs
	// Called when using SimpleHttp for offline downloading, skipping creating a download task
	PutURL(ctx context.Context, dstDir model.Obj, name, url string) (model.Obj, error)
}

type ArchiveReader interface {
	// GetArchiveMeta get the meta-info of an archive
	// return errs.WrongArchivePassword if the meta-info is also encrypted but provided password is wrong or empty
	// return errs.NotImplement to use internal archive tools to get the meta-info, such as the following cases:
	// 1. the driver do not support the format of the archive but there may be an internal tool do
	// 2. handling archives is a VIP feature, but the driver does not have VIP access
	GetArchiveMeta(ctx context.Context, obj model.Obj, args model.ArchiveArgs) (model.ArchiveMeta, error)
	// ListArchive list the children of model.ArchiveArgs.InnerPath in the archive
	// return errs.NotImplement to use internal archive tools to list the children
	// return errs.NotSupport if the folder structure should be acquired from model.ArchiveMeta.GetTree
	ListArchive(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) ([]model.Obj, error)
	// Extract get url/filepath/reader of a file in the archive
	// return errs.NotImplement to use internal archive tools to extract
	Extract(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (*model.Link, error)
}

type ArchiveGetter interface {
	// ArchiveGet get file by inner path
	// return errs.NotImplement to use internal archive tools to get the children
	// return errs.NotSupport if the folder structure should be acquired from model.ArchiveMeta.GetTree
	ArchiveGet(ctx context.Context, obj model.Obj, args model.ArchiveInnerArgs) (model.Obj, error)
}

type ArchiveDecompress interface {
	ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) error
}

type ArchiveDecompressResult interface {
	// ArchiveDecompress decompress an archive
	// when args.PutIntoNewDir, the new sub-folder should be named the same to the archive but without the extension
	// return each decompressed obj from the root path of the archive when args.PutIntoNewDir is false
	// return only the newly created folder when args.PutIntoNewDir is true
	// return errs.NotImplement to use internal archive tools to decompress
	ArchiveDecompress(ctx context.Context, srcObj, dstDir model.Obj, args model.ArchiveDecompressArgs) ([]model.Obj, error)
}

type Reference interface {
	InitReference(storage Driver) error
}
