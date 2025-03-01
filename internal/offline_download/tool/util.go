package tool

import (
	"os"
	"path/filepath"
)

func GetFiles(dir string) ([]File, error) {
	var files []File
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, File{
				Name:     info.Name(),
				Size:     info.Size(),
				Path:     path,
				Modified: info.ModTime(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func GetFile(path string) (File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	return File{
		Name:     info.Name(),
		Size:     info.Size(),
		Path:     path,
		Modified: info.ModTime(),
	}, nil
}
