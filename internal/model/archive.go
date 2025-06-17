package model

import "time"

type ObjTree interface {
	Obj
	GetChildren() []ObjTree
}

type ObjectTree struct {
	Object
	Children []ObjTree
}

func (t *ObjectTree) GetChildren() []ObjTree {
	return t.Children
}

type ArchiveMeta interface {
	GetComment() string
	// IsEncrypted means if the content of the archive requires a password to access
	// GetArchiveMeta should return errs.WrongArchivePassword if the meta-info is also encrypted,
	// and the provided password is empty.
	IsEncrypted() bool
	// GetTree directly returns the full folder structure
	// returns nil if the folder structure should be acquired by calling driver.ArchiveReader.ListArchive
	GetTree() []ObjTree
}

type ArchiveMetaInfo struct {
	Comment   string
	Encrypted bool
	Tree      []ObjTree
}

func (m *ArchiveMetaInfo) GetComment() string {
	return m.Comment
}

func (m *ArchiveMetaInfo) IsEncrypted() bool {
	return m.Encrypted
}

func (m *ArchiveMetaInfo) GetTree() []ObjTree {
	return m.Tree
}

type ArchiveMetaProvider struct {
	ArchiveMeta
	*Sort
	DriverProviding bool
	Expiration      *time.Duration
}
