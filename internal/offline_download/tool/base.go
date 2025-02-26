package tool

import (
	"io"
	"os"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
)

type AddUrlArgs struct {
	Url     string
	UID     string
	TempDir string
	Signal  chan int
}

type Status struct {
	TotalBytes int64
	Progress   float64
	NewGID     string
	Completed  bool
	Status     string
	Err        error
}

type Tool interface {
	Name() string
	// Items return the setting items the tool need
	Items() []model.SettingItem
	Init() (string, error)
	IsReady() bool
	// AddURL add an uri to download, return the task id
	AddURL(args *AddUrlArgs) (string, error)
	// Remove the download if task been canceled
	Remove(task *DownloadTask) error
	// Status return the status of the download task, if an error occurred, return the error in Status.Err
	Status(task *DownloadTask) (*Status, error)

	// Run for simple http download
	Run(task *DownloadTask) error
}

type GetFileser interface {
	// GetFiles return the files of the download task, if nil, means walk the temp dir to get the files
	GetFiles(task *DownloadTask) []File
}

type File struct {
	// ReadCloser for http client
	ReadCloser io.ReadCloser
	Name       string
	Size       int64
	Path       string
	Modified   time.Time
}

func (f *File) GetReadCloser() (io.ReadCloser, error) {
	if f.ReadCloser != nil {
		return f.ReadCloser, nil
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	return file, nil
}
