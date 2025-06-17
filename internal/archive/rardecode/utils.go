package rardecode

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/nwaples/rardecode/v2"
	"io"
	"io/fs"
	"os"
	stdpath "path"
	"sort"
	"strings"
	"time"
)

type VolumeFile struct {
	stream.SStreamReadAtSeeker
	name string
}

func (v *VolumeFile) Name() string {
	return v.name
}

func (v *VolumeFile) Size() int64 {
	return v.SStreamReadAtSeeker.GetRawStream().GetSize()
}

func (v *VolumeFile) Mode() fs.FileMode {
	return 0644
}

func (v *VolumeFile) ModTime() time.Time {
	return v.SStreamReadAtSeeker.GetRawStream().ModTime()
}

func (v *VolumeFile) IsDir() bool {
	return false
}

func (v *VolumeFile) Sys() any {
	return nil
}

func (v *VolumeFile) Stat() (fs.FileInfo, error) {
	return v, nil
}

func (v *VolumeFile) Close() error {
	return nil
}

type VolumeFs struct {
	parts map[string]*VolumeFile
}

func (v *VolumeFs) Open(name string) (fs.File, error) {
	file, ok := v.parts[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return file, nil
}

func makeOpts(ss []*stream.SeekableStream) (string, rardecode.Option, error) {
	if len(ss) == 1 {
		reader, err := stream.NewReadAtSeeker(ss[0], 0)
		if err != nil {
			return "", nil, err
		}
		fileName := "file.rar"
		fsys := &VolumeFs{parts: map[string]*VolumeFile{
			fileName: {SStreamReadAtSeeker: reader, name: fileName},
		}}
		return fileName, rardecode.FileSystem(fsys), nil
	} else {
		parts := make(map[string]*VolumeFile, len(ss))
		for i, s := range ss {
			reader, err := stream.NewReadAtSeeker(s, 0)
			if err != nil {
				return "", nil, err
			}
			fileName := fmt.Sprintf("file.part%d.rar", i+1)
			parts[fileName] = &VolumeFile{SStreamReadAtSeeker: reader, name: fileName}
		}
		return "file.part1.rar", rardecode.FileSystem(&VolumeFs{parts: parts}), nil
	}
}

type WrapReader struct {
	files []*rardecode.File
}

func (r *WrapReader) Files() []tool.SubFile {
	ret := make([]tool.SubFile, 0, len(r.files))
	for _, f := range r.files {
		ret = append(ret, &WrapFile{File: f})
	}
	return ret
}

type WrapFile struct {
	*rardecode.File
}

func (f *WrapFile) Name() string {
	if f.File.IsDir {
		return f.File.Name + "/"
	}
	return f.File.Name
}

func (f *WrapFile) FileInfo() fs.FileInfo {
	return &WrapFileInfo{File: f.File}
}

type WrapFileInfo struct {
	*rardecode.File
}

func (f *WrapFileInfo) Name() string {
	return stdpath.Base(f.File.Name)
}

func (f *WrapFileInfo) Size() int64 {
	return f.File.UnPackedSize
}

func (f *WrapFileInfo) ModTime() time.Time {
	return f.File.ModificationTime
}

func (f *WrapFileInfo) IsDir() bool {
	return f.File.IsDir
}

func (f *WrapFileInfo) Sys() any {
	return nil
}

func list(ss []*stream.SeekableStream, password string) (*WrapReader, error) {
	fileName, fsOpt, err := makeOpts(ss)
	if err != nil {
		return nil, err
	}
	opts := []rardecode.Option{fsOpt}
	if password != "" {
		opts = append(opts, rardecode.Password(password))
	}
	files, err := rardecode.List(fileName, opts...)
	// rardecode输出文件列表的顺序不一定是父目录在前，子目录在后
	// 父路径的长度一定比子路径短，排序后的files可保证父路径在前
	sort.Slice(files, func(i, j int) bool {
		return len(files[i].Name) < len(files[j].Name)
	})
	if err != nil {
		return nil, filterPassword(err)
	}
	return &WrapReader{files: files}, nil
}

func getReader(ss []*stream.SeekableStream, password string) (*rardecode.Reader, error) {
	fileName, fsOpt, err := makeOpts(ss)
	if err != nil {
		return nil, err
	}
	opts := []rardecode.Option{fsOpt}
	if password != "" {
		opts = append(opts, rardecode.Password(password))
	}
	rc, err := rardecode.OpenReader(fileName, opts...)
	if err != nil {
		return nil, filterPassword(err)
	}
	ss[0].Closers.Add(rc)
	return &rc.Reader, nil
}

func decompress(reader *rardecode.Reader, header *rardecode.FileHeader, filePath, outputPath string) error {
	targetPath := outputPath
	dir, base := stdpath.Split(filePath)
	if dir != "" {
		targetPath = stdpath.Join(targetPath, dir)
		err := os.MkdirAll(targetPath, 0700)
		if err != nil {
			return err
		}
	}
	if base != "" {
		err := _decompress(reader, header, targetPath, func(_ float64) {})
		if err != nil {
			return err
		}
	}
	return nil
}

func _decompress(reader *rardecode.Reader, header *rardecode.FileHeader, targetPath string, up model.UpdateProgress) error {
	f, err := os.OpenFile(stdpath.Join(targetPath, stdpath.Base(header.Name)), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, &stream.ReaderUpdatingProgress{
		Reader: &stream.SimpleReaderWithSize{
			Reader: reader,
			Size:   header.UnPackedSize,
		},
		UpdateProgress: up,
	})
	if err != nil {
		return err
	}
	return nil
}

func filterPassword(err error) error {
	if err != nil && strings.Contains(err.Error(), "password") {
		return errs.WrongArchivePassword
	}
	return err
}
