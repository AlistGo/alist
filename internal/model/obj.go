package model

import (
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/dlclark/regexp2"

	mapset "github.com/deckarep/golang-set/v2"

	"github.com/maruel/natural"
)

type ObjUnwrap interface {
	Unwrap() Obj
}

type Obj interface {
	GetSize() int64
	GetName() string
	ModTime() time.Time
	CreateTime() time.Time
	IsDir() bool
	GetHash() utils.HashInfo

	// The internal information of the driver.
	// If you want to use it, please understand what it means
	GetID() string
	GetPath() string
}

// FileStreamer ->check FileStream for more comments
type FileStreamer interface {
	io.Reader
	io.Closer
	Obj
	GetMimetype() string
	//SetReader(io.Reader)
	NeedStore() bool
	IsForceStreamUpload() bool
	GetExist() Obj
	SetExist(Obj)
	//for a non-seekable Stream, RangeRead supports peeking some data, and CacheFullInTempFile still works
	RangeRead(http_range.Range) (io.Reader, error)
	//for a non-seekable Stream, if Read is called, this function won't work
	CacheFullInTempFile() (File, error)
	SetTmpFile(r *os.File)
	GetFile() File
}

type UpdateProgress func(percentage float64)

type URL interface {
	URL() string
}

type Thumb interface {
	Thumb() string
}

type SetPath interface {
	SetPath(path string)
}

func SortFiles(objs []Obj, orderBy, orderDirection string) {
	if orderBy == "" {
		return
	}
	sort.Slice(objs, func(i, j int) bool {
		switch orderBy {
		case "name":
			{
				c := natural.Less(objs[i].GetName(), objs[j].GetName())
				if orderDirection == "desc" {
					return !c
				}
				return c
			}
		case "size":
			{
				if orderDirection == "desc" {
					return objs[i].GetSize() >= objs[j].GetSize()
				}
				return objs[i].GetSize() <= objs[j].GetSize()
			}
		case "modified":
			if orderDirection == "desc" {
				return objs[i].ModTime().After(objs[j].ModTime())
			}
			return objs[i].ModTime().Before(objs[j].ModTime())
		}
		return false
	})
}

func ExtractFolder(objs []Obj, extractFolder string) {
	if extractFolder == "" {
		return
	}
	front := extractFolder == "front"
	sort.SliceStable(objs, func(i, j int) bool {
		if objs[i].IsDir() || objs[j].IsDir() {
			if !objs[i].IsDir() {
				return !front
			}
			if !objs[j].IsDir() {
				return front
			}
		}
		return false
	})
}

func WrapObjName(objs Obj) Obj {
	return &ObjWrapName{Name: utils.MappingName(objs.GetName()), Obj: objs}
}

func WrapObjsName(objs []Obj) {
	for i := 0; i < len(objs); i++ {
		objs[i] = &ObjWrapName{Name: utils.MappingName(objs[i].GetName()), Obj: objs[i]}
	}
}

func UnwrapObj(obj Obj) Obj {
	if unwrap, ok := obj.(ObjUnwrap); ok {
		obj = unwrap.Unwrap()
	}
	return obj
}

func GetThumb(obj Obj) (thumb string, ok bool) {
	if obj, ok := obj.(Thumb); ok {
		return obj.Thumb(), true
	}
	if unwrap, ok := obj.(ObjUnwrap); ok {
		return GetThumb(unwrap.Unwrap())
	}
	return thumb, false
}

func GetUrl(obj Obj) (url string, ok bool) {
	if obj, ok := obj.(URL); ok {
		return obj.URL(), true
	}
	if unwrap, ok := obj.(ObjUnwrap); ok {
		return GetUrl(unwrap.Unwrap())
	}
	return url, false
}

func GetRawObject(obj Obj) *Object {
	switch v := obj.(type) {
	case *ObjThumbURL:
		return &v.Object
	case *ObjThumb:
		return &v.Object
	case *ObjectURL:
		return &v.Object
	case *Object:
		return v
	}
	return nil
}

// Merge
func NewObjMerge() *ObjMerge {
	return &ObjMerge{
		set: mapset.NewSet[string](),
	}
}

type ObjMerge struct {
	regs []*regexp2.Regexp
	set  mapset.Set[string]
}

func (om *ObjMerge) Merge(objs []Obj, objs_ ...Obj) []Obj {
	newObjs := make([]Obj, 0, len(objs)+len(objs_))
	newObjs = om.insertObjs(om.insertObjs(newObjs, objs...), objs_...)
	return newObjs
}

func (om *ObjMerge) insertObjs(objs []Obj, objs_ ...Obj) []Obj {
	for _, obj := range objs_ {
		if om.clickObj(obj) {
			objs = append(objs, obj)
		}
	}
	return objs
}

func (om *ObjMerge) clickObj(obj Obj) bool {
	for _, reg := range om.regs {
		if isMatch, _ := reg.MatchString(obj.GetName()); isMatch {
			return false
		}
	}
	return om.set.Add(obj.GetName())
}

func (om *ObjMerge) InitHideReg(hides string) {
	rs := strings.Split(hides, "\n")
	om.regs = make([]*regexp2.Regexp, 0, len(rs))
	for _, r := range rs {
		om.regs = append(om.regs, regexp2.MustCompile(r, regexp2.None))
	}
}

func (om *ObjMerge) Reset() {
	om.set.Clear()
}
