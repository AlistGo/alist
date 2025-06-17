package zip

import (
	"io"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
)

type Zip struct {
}

func (Zip) AcceptedExtensions() []string {
	return []string{}
}

func (Zip) AcceptedMultipartExtensions() map[string]tool.MultipartExtension {
	return map[string]tool.MultipartExtension{
		".zip":     {".z%.2d", 1},
		".zip.001": {".zip.%.3d", 2},
	}
}

func (Zip) GetMeta(ss []*stream.SeekableStream, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	zipReader, err := getReader(ss)
	if err != nil {
		return nil, err
	}
	encrypted, tree := tool.GenerateMetaTreeFromFolderTraversal(&WrapReader{Reader: zipReader})
	return &model.ArchiveMetaInfo{
		Comment:   zipReader.Comment,
		Encrypted: encrypted,
		Tree:      tree,
	}, nil
}

func (Zip) List(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	zipReader, err := getReader(ss)
	if err != nil {
		return nil, err
	}
	if args.InnerPath == "/" {
		ret := make([]model.Obj, 0)
		passVerified := false
		var dir *model.Object
		for _, file := range zipReader.File {
			if !passVerified && file.IsEncrypted() {
				file.SetPassword(args.Password)
				rc, e := file.Open()
				if e != nil {
					return nil, filterPassword(e)
				}
				_ = rc.Close()
				passVerified = true
			}
			name := strings.TrimSuffix(decodeName(file.Name), "/")
			if strings.Contains(name, "/") {
				// 有些压缩包不压缩第一个文件夹
				strs := strings.Split(name, "/")
				if dir == nil && len(strs) == 2 {
					dir = &model.Object{
						Name:     strs[0],
						Modified: ss[0].ModTime(),
						IsFolder: true,
					}
				}
				continue
			}
			ret = append(ret, tool.MakeModelObj(&WrapFileInfo{FileInfo: file.FileInfo()}))
		}
		if len(ret) == 0 && dir != nil {
			ret = append(ret, dir)
		}
		return ret, nil
	} else {
		innerPath := strings.TrimPrefix(args.InnerPath, "/") + "/"
		ret := make([]model.Obj, 0)
		exist := false
		for _, file := range zipReader.File {
			name := decodeName(file.Name)
			dir := stdpath.Dir(strings.TrimSuffix(name, "/")) + "/"
			if dir != innerPath {
				continue
			}
			exist = true
			ret = append(ret, tool.MakeModelObj(&WrapFileInfo{file.FileInfo()}))
		}
		if !exist {
			return nil, errs.ObjectNotFound
		}
		return ret, nil
	}
}

func (Zip) Extract(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	zipReader, err := getReader(ss)
	if err != nil {
		return nil, 0, err
	}
	innerPath := strings.TrimPrefix(args.InnerPath, "/")
	for _, file := range zipReader.File {
		if decodeName(file.Name) == innerPath {
			if file.IsEncrypted() {
				file.SetPassword(args.Password)
			}
			r, e := file.Open()
			if e != nil {
				return nil, 0, e
			}
			return r, file.FileInfo().Size(), nil
		}
	}
	return nil, 0, errs.ObjectNotFound
}

func (Zip) Decompress(ss []*stream.SeekableStream, outputPath string, args model.ArchiveInnerArgs, up model.UpdateProgress) error {
	zipReader, err := getReader(ss)
	if err != nil {
		return err
	}
	return tool.DecompressFromFolderTraversal(&WrapReader{Reader: zipReader}, outputPath, args, up)
}

var _ tool.Tool = (*Zip)(nil)

func init() {
	tool.RegisterTool(Zip{})
}
