package tool

import (
	"github.com/alist-org/alist/v3/internal/errs"
)

var (
	Tools               = make(map[string]Tool)
	MultipartExtensions = make(map[string]MultipartExtension)
)

func RegisterTool(tool Tool) {
	for _, ext := range tool.AcceptedExtensions() {
		Tools[ext] = tool
	}
	for mainFile, ext := range tool.AcceptedMultipartExtensions() {
		MultipartExtensions[mainFile] = ext
		Tools[mainFile] = tool
	}
}

func GetArchiveTool(ext string) (*MultipartExtension, Tool, error) {
	t, ok := Tools[ext]
	if !ok {
		return nil, nil, errs.UnknownArchiveFormat
	}
	partExt, ok := MultipartExtensions[ext]
	if !ok {
		return nil, t, nil
	}
	return &partExt, t, nil
}
