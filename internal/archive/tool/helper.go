package tool

import (
	"io"
	"io/fs"
	"os"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
)

type SubFile interface {
	Name() string
	FileInfo() fs.FileInfo
	Open() (io.ReadCloser, error)
}

type CanEncryptSubFile interface {
	IsEncrypted() bool
	SetPassword(password string)
}

type ArchiveReader interface {
	Files() []SubFile
}

func GenerateMetaTreeFromFolderTraversal(r ArchiveReader) (bool, []model.ObjTree) {
	encrypted := false
	dirMap := make(map[string]*model.ObjectTree)
	for _, file := range r.Files() {
		if encrypt, ok := file.(CanEncryptSubFile); ok && encrypt.IsEncrypted() {
			encrypted = true
		}

		name := strings.TrimPrefix(file.Name(), "/")
		var dir string
		var dirObj *model.ObjectTree
		isNewFolder := false
		if !file.FileInfo().IsDir() {
			// 先将 文件 添加到 所在的文件夹
			dir = stdpath.Dir(name)
			dirObj = dirMap[dir]
			if dirObj == nil {
				isNewFolder = dir != "."
				dirObj = &model.ObjectTree{}
				dirObj.IsFolder = true
				dirObj.Name = stdpath.Base(dir)
				dirObj.Modified = file.FileInfo().ModTime()
				dirMap[dir] = dirObj
			}
			dirObj.Children = append(
				dirObj.Children, &model.ObjectTree{
					Object: *MakeModelObj(file.FileInfo()),
				},
			)
		} else {
			dir = strings.TrimSuffix(name, "/")
			dirObj = dirMap[dir]
			if dirObj == nil {
				isNewFolder = dir != "."
				dirObj = &model.ObjectTree{}
				dirMap[dir] = dirObj
			}
			dirObj.IsFolder = true
			dirObj.Name = stdpath.Base(dir)
			dirObj.Modified = file.FileInfo().ModTime()
		}
		if isNewFolder {
			// 将 文件夹 添加到 父文件夹
			// 考虑压缩包仅记录文件的路径，不记录文件夹
			// 循环创建所有父文件夹
			parentDir := stdpath.Dir(dir)
			for {
				parentDirObj := dirMap[parentDir]
				if parentDirObj == nil {
					parentDirObj = &model.ObjectTree{}
					if parentDir != "." {
						parentDirObj.IsFolder = true
						parentDirObj.Name = stdpath.Base(parentDir)
						parentDirObj.Modified = file.FileInfo().ModTime()
					}
					dirMap[parentDir] = parentDirObj
				}
				parentDirObj.Children = append(parentDirObj.Children, dirObj)

				parentDir = stdpath.Dir(parentDir)
				if dirMap[parentDir] != nil {
					break
				}
				dirObj = parentDirObj
			}
		}
	}
	if len(dirMap) > 0 {
		return encrypted, dirMap["."].GetChildren()
	} else {
		return encrypted, nil
	}
}

func MakeModelObj(file os.FileInfo) *model.Object {
	return &model.Object{
		Name:     file.Name(),
		Size:     file.Size(),
		Modified: file.ModTime(),
		IsFolder: file.IsDir(),
	}
}

type WrapFileInfo struct {
	model.Obj
}

func DecompressFromFolderTraversal(r ArchiveReader, outputPath string, args model.ArchiveInnerArgs, up model.UpdateProgress) error {
	var err error
	files := r.Files()
	if args.InnerPath == "/" {
		for i, file := range files {
			name := file.Name()
			err = decompress(file, name, outputPath, args.Password)
			if err != nil {
				return err
			}
			up(float64(i+1) * 100.0 / float64(len(files)))
		}
	} else {
		innerPath := strings.TrimPrefix(args.InnerPath, "/")
		innerBase := stdpath.Base(innerPath)
		createdBaseDir := false
		for _, file := range files {
			name := file.Name()
			if name == innerPath {
				err = _decompress(file, outputPath, args.Password, up)
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
				err = decompress(file, restPath, targetPath, args.Password)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func decompress(file SubFile, filePath, outputPath, password string) error {
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
		err := _decompress(file, targetPath, password, func(_ float64) {})
		if err != nil {
			return err
		}
	}
	return nil
}

func _decompress(file SubFile, targetPath, password string, up model.UpdateProgress) error {
	if encrypt, ok := file.(CanEncryptSubFile); ok && encrypt.IsEncrypted() {
		encrypt.SetPassword(password)
	}
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	f, err := os.OpenFile(stdpath.Join(targetPath, file.FileInfo().Name()), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, &stream.ReaderUpdatingProgress{
		Reader: &stream.SimpleReaderWithSize{
			Reader: rc,
			Size:   file.FileInfo().Size(),
		},
		UpdateProgress: up,
	})
	if err != nil {
		return err
	}
	return nil
}
