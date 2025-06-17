package iso9660

import (
	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/kdomanski/iso9660"
	"io"
	"os"
	stdpath "path"
)

type ISO9660 struct {
}

func (ISO9660) AcceptedExtensions() []string {
	return []string{".iso"}
}

func (ISO9660) AcceptedMultipartExtensions() map[string]tool.MultipartExtension {
	return map[string]tool.MultipartExtension{}
}

func (ISO9660) GetMeta(ss []*stream.SeekableStream, args model.ArchiveArgs) (model.ArchiveMeta, error) {
	return &model.ArchiveMetaInfo{
		Comment:   "",
		Encrypted: false,
	}, nil
}

func (ISO9660) List(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) ([]model.Obj, error) {
	img, err := getImage(ss[0])
	if err != nil {
		return nil, err
	}
	dir, err := getObj(img, args.InnerPath)
	if err != nil {
		return nil, err
	}
	if !dir.IsDir() {
		return nil, errs.NotFolder
	}
	children, err := dir.GetChildren()
	if err != nil {
		return nil, err
	}
	ret := make([]model.Obj, 0, len(children))
	for _, child := range children {
		ret = append(ret, toModelObj(child))
	}
	return ret, nil
}

func (ISO9660) Extract(ss []*stream.SeekableStream, args model.ArchiveInnerArgs) (io.ReadCloser, int64, error) {
	img, err := getImage(ss[0])
	if err != nil {
		return nil, 0, err
	}
	obj, err := getObj(img, args.InnerPath)
	if err != nil {
		return nil, 0, err
	}
	if obj.IsDir() {
		return nil, 0, errs.NotFile
	}
	return io.NopCloser(obj.Reader()), obj.Size(), nil
}

func (ISO9660) Decompress(ss []*stream.SeekableStream, outputPath string, args model.ArchiveInnerArgs, up model.UpdateProgress) error {
	img, err := getImage(ss[0])
	if err != nil {
		return err
	}
	obj, err := getObj(img, args.InnerPath)
	if err != nil {
		return err
	}
	if obj.IsDir() {
		if args.InnerPath != "/" {
			outputPath = stdpath.Join(outputPath, obj.Name())
			if err = os.MkdirAll(outputPath, 0700); err != nil {
				return err
			}
		}
		var children []*iso9660.File
		if children, err = obj.GetChildren(); err == nil {
			err = decompressAll(children, outputPath)
		}
	} else {
		err = decompress(obj, outputPath, up)
	}
	return err
}

var _ tool.Tool = (*ISO9660)(nil)

func init() {
	tool.RegisterTool(ISO9660{})
}
