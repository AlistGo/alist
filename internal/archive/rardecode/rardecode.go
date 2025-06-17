package rardecode

import (
	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/nwaples/rardecode/v2"
	"io"
	"os"
	stdpath "path"
	"strings"
)

type RarDecoder struct{}

func (RarDecoder) AcceptedExtensions() []string {
	return []string{".rar"}
}

func (RarDecoder) AcceptedMultipartExtensions() map[string]tool.MultipartExtension {
	return map[string]tool.MultipartExtension{
		".part1.rar": {".part%d.rar", 2},
	}
}

func (RarDecoder) GetMeta(ss []*stream.SeekableStream, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	l, err := list(ss, args.Password)
	if err != nil {
		return nil, err
	}
	_, tree := tool.GenerateMetaTreeFromFolderTraversal(l)
	return &model.ArchiveMetaInfo{
		Comment:   "",
		Encrypted: false,
		Tree:      tree,
	}, nil
}

func (RarDecoder) List(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	return nil, errs.NotSupport
}

func (RarDecoder) Extract(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	reader, err := getReader(ss, args.Password)
	if err != nil {
		return nil, 0, err
	}
	innerPath := strings.TrimPrefix(args.InnerPath, "/")
	for {
		var header *rardecode.FileHeader
		header, err = reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if header.Name == innerPath {
			if header.IsDir {
				break
			}
			return io.NopCloser(reader), header.UnPackedSize, nil
		}
	}
	return nil, 0, errs.ObjectNotFound
}

func (RarDecoder) Decompress(ss []*stream.SeekableStream, outputPath string, args model.ArchiveInnerArgs, up model.UpdateProgress) error {
	reader, err := getReader(ss, args.Password)
	if err != nil {
		return err
	}
	if args.InnerPath == "/" {
		for {
			var header *rardecode.FileHeader
			header, err = reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			name := header.Name
			if header.IsDir {
				name = name + "/"
			}
			err = decompress(reader, header, name, outputPath)
			if err != nil {
				return err
			}
		}
	} else {
		innerPath := strings.TrimPrefix(args.InnerPath, "/")
		innerBase := stdpath.Base(innerPath)
		createdBaseDir := false
		for {
			var header *rardecode.FileHeader
			header, err = reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			name := header.Name
			if header.IsDir {
				name = name + "/"
			}
			if name == innerPath {
				err = _decompress(reader, header, outputPath, up)
				if err != nil {
					return err
				}
				break
			} else if strings.HasPrefix(name, innerPath+"/") {
				targetPath := stdpath.Join(outputPath, innerBase)
				if !createdBaseDir {
					err = os.Mkdir(targetPath, 0700)
					if err != nil {
						return err
					}
					createdBaseDir = true
				}
				restPath := strings.TrimPrefix(name, innerPath+"/")
				err = decompress(reader, header, restPath, targetPath)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

var _ tool.Tool = (*RarDecoder)(nil)

func init() {
	tool.RegisterTool(RarDecoder{})
}
