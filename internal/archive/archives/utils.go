package archives

import (
	"io"
	fs2 "io/fs"
	"os"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/mholt/archives"
)

func getFs(ss *stream.SeekableStream, args model.ArchiveArgs) (*archives.ArchiveFS, error) {
	reader, err := stream.NewReadAtSeeker(ss, 0)
	if err != nil {
		return nil, err
	}
	if r, ok := reader.(*stream.RangeReadReadAtSeeker); ok {
		r.InitHeadCache()
	}
	format, _, err := archives.Identify(ss.Ctx, ss.GetName(), reader)
	if err != nil {
		return nil, errs.UnknownArchiveFormat
	}
	extractor, ok := format.(archives.Extractor)
	if !ok {
		return nil, errs.UnknownArchiveFormat
	}
	switch f := format.(type) {
	case archives.SevenZip:
		f.Password = args.Password
	case archives.Rar:
		f.Password = args.Password
	}
	return &archives.ArchiveFS{
		Stream:  io.NewSectionReader(reader, 0, ss.GetSize()),
		Format:  extractor,
		Context: ss.Ctx,
	}, nil
}

func toModelObj(file os.FileInfo) *model.Object {
	return &model.Object{
		Name:     file.Name(),
		Size:     file.Size(),
		Modified: file.ModTime(),
		IsFolder: file.IsDir(),
	}
}

func filterPassword(err error) error {
	if err != nil && strings.Contains(err.Error(), "password") {
		return errs.WrongArchivePassword
	}
	return err
}

func decompress(fsys fs2.FS, filePath, targetPath string, up model.UpdateProgress) error {
	rc, err := fsys.Open(filePath)
	if err != nil {
		return err
	}
	defer rc.Close()
	stat, err := rc.Stat()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(stdpath.Join(targetPath, stat.Name()), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = utils.CopyWithBuffer(f, &stream.ReaderUpdatingProgress{
		Reader: &stream.SimpleReaderWithSize{
			Reader: rc,
			Size:   stat.Size(),
		},
		UpdateProgress: up,
	})
	return err
}
