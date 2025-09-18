package protondrive

/*
Package protondrive
Author: Da3zKi7<da3zki7@duck.com>
Date: 2025-09-18

Thanks to @henrybear327 for modded go-proton-api & Proton-API-Bridge

The power of open-source, the force of teamwork and the magic of reverse engineering!


D@' 3z K!7 - The King Of Cracking

Да здравствует Родина))
*/

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/henrybear327/go-proton-api"
)

type ProtonFile struct {
	*proton.Link
	Name     string
	IsFolder bool
}

func (p *ProtonFile) GetName() string {
	return p.Name
}

func (p *ProtonFile) GetSize() int64 {
	return p.Link.Size
}

func (p *ProtonFile) GetPath() string {
	return p.Name
}

func (p *ProtonFile) IsDir() bool {
	return p.IsFolder
}

func (p *ProtonFile) ModTime() time.Time {
	return time.Unix(p.Link.ModifyTime, 0)
}

func (p *ProtonFile) CreateTime() time.Time {
	return time.Unix(p.Link.CreateTime, 0)
}

type downloadInfo struct {
	LinkID   string
	FileName string
}

type fileStreamer struct {
	io.ReadCloser
	obj model.Obj
}

func (fs *fileStreamer) GetMimetype() string       { return "" }
func (fs *fileStreamer) NeedStore() bool           { return false }
func (fs *fileStreamer) IsForceStreamUpload() bool { return false }
func (fs *fileStreamer) GetExist() model.Obj       { return nil }
func (fs *fileStreamer) SetExist(model.Obj)        {}
func (fs *fileStreamer) RangeRead(http_range.Range) (io.Reader, error) {
	return nil, errors.New("not supported")
}
func (fs *fileStreamer) CacheFullInTempFile() (model.File, error) {
	return nil, errors.New("not supported")
}
func (fs *fileStreamer) SetTmpFile(r *os.File)   {}
func (fs *fileStreamer) GetFile() model.File     { return nil }
func (fs *fileStreamer) GetName() string         { return fs.obj.GetName() }
func (fs *fileStreamer) GetSize() int64          { return fs.obj.GetSize() }
func (fs *fileStreamer) GetPath() string         { return fs.obj.GetPath() }
func (fs *fileStreamer) IsDir() bool             { return fs.obj.IsDir() }
func (fs *fileStreamer) ModTime() time.Time      { return fs.obj.ModTime() }
func (fs *fileStreamer) CreateTime() time.Time   { return fs.obj.ModTime() }
func (fs *fileStreamer) GetHash() utils.HashInfo { return fs.obj.GetHash() }
func (fs *fileStreamer) GetID() string           { return fs.obj.GetID() }

type httpRange struct {
	start, end int64
}

type MoveRequest struct {
	ParentLinkID            string  `json:"ParentLinkID"`
	NodePassphrase          string  `json:"NodePassphrase"`
	NodePassphraseSignature *string `json:"NodePassphraseSignature"`
	Name                    string  `json:"Name"`
	NameSignatureEmail      string  `json:"NameSignatureEmail"`
	Hash                    string  `json:"Hash"`
	OriginalHash            string  `json:"OriginalHash"`
	ContentHash             *string `json:"ContentHash"` // Maybe null
}

type progressReader struct {
	reader   io.Reader
	total    int64
	current  int64
	callback driver.UpdateProgress
}

type RenameRequest struct {
	Name               string `json:"Name"`               // PGP encrypted name
	NameSignatureEmail string `json:"NameSignatureEmail"` // User's signature email
	Hash               string `json:"Hash"`               // New name hash
	OriginalHash       string `json:"OriginalHash"`       // Current name hash
}

type RenameResponse struct {
	Code int `json:"Code"`
}
