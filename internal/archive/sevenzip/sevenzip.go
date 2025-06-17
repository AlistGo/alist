package sevenzip

import (
	"io"
	"strings"

	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
)

type SevenZip struct{}

func (SevenZip) AcceptedExtensions() []string {
	return []string{".7z"}
}

func (SevenZip) AcceptedMultipartExtensions() map[string]tool.MultipartExtension {
	return map[string]tool.MultipartExtension{
		".7z.001": {".7z.%.3d", 2},
	}
}

func (SevenZip) GetMeta(ss []*stream.SeekableStream, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	reader, err := getReader(ss, args.Password)
	if err != nil {
		return nil, err
	}
	_, tree := tool.GenerateMetaTreeFromFolderTraversal(&WrapReader{Reader: reader})
	return &model.ArchiveMetaInfo{
		Comment:   "",
		Encrypted: args.Password != "",
		Tree:      tree,
	}, nil
}

func (SevenZip) List(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotSupport
}

func (SevenZip) Extract(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	reader, err := getReader(ss, args.Password)
	if err != nil {
		return nil, 0, err
	}
	innerPath := strings.TrimPrefix(args.InnerPath, "/")
	for _, file := range reader.File {
		if file.Name == innerPath {
			r, e := file.Open()
			if e != nil {
				return nil, 0, e
			}
			return r, file.FileInfo().Size(), nil
		}
	}
	return nil, 0, errs.ObjectNotFound
}

func (SevenZip) Decompress(ss []*stream.SeekableStream, outputPath string, args model.ArchiveInnerArgs, up model.UpdateProgress) error {
	reader, err := getReader(ss, args.Password)
	if err != nil {
		return err
	}
	return tool.DecompressFromFolderTraversal(&WrapReader{Reader: reader}, outputPath, args, up)
}

var _ tool.Tool = (*SevenZip)(nil)

func init() {
	tool.RegisterTool(SevenZip{})
}
