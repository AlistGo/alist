package tool

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/errs"
)

var (
	Tools               = make(map[string]Tool)
	MultipartExtensions = make(map[string]string)
)

func RegisterTool(tool Tool) {
	for _, ext := range tool.AcceptedExtensions() {
		Tools[ext] = tool
	}
	for _, ext := range tool.AcceptedMultipartExtensions() {
		first := fmt.Sprintf(ext, 1)
		MultipartExtensions[first] = ext
		Tools[first] = tool
	}
}

func GetArchiveTool(ext string) (string, Tool, error) {
	t, ok := Tools[ext]
	if !ok {
		return "", nil, errs.UnknownArchiveFormat
	}
	partExt, ok := MultipartExtensions[ext]
	if !ok {
		return "", t, nil
	}
	return partExt, t, nil
}
