package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"iter"

	"github.com/alist-org/alist/v3/internal/errs"
	log "github.com/sirupsen/logrus"
)

func GetMD5EncodeStr(data string) string {
	return HashData(MD5, []byte(data))
}

//inspired by "github.com/rclone/rclone/fs/hash"

// ErrUnsupported should be returned by filesystem,
// if it is requested to deliver an unsupported hash type.
var ErrUnsupported = errors.New("hash type not supported")

// HashType indicates a standard hashing algorithm
type HashType struct {
	Width   int
	Name    string
	Alias   string
	NewFunc func(...any) hash.Hash
}

func (ht *HashType) MarshalJSON() ([]byte, error) {
	return []byte(`"` + ht.Name + `"`), nil
}

func (ht *HashType) MarshalText() (text []byte, err error) {
	return []byte(ht.Name), nil
}

var (
	_ json.Marshaler = (*HashType)(nil)
	//_ json.Unmarshaler = (*HashType)(nil)

	// read/write from/to json keys
	_ encoding.TextMarshaler = (*HashType)(nil)
	//_ encoding.TextUnmarshaler = (*HashType)(nil)
)

var (
	name2hash  = map[string]*HashType{}
	alias2hash = map[string]*HashType{}
	Supported  []*HashType
)

// RegisterHash adds a new Hash to the list and returns its Type
func RegisterHash(name, alias string, width int, newFunc func() hash.Hash) *HashType {
	return RegisterHashWithParam(name, alias, width, func(a ...any) hash.Hash { return newFunc() })
}

func RegisterHashWithParam(name, alias string, width int, newFunc func(...any) hash.Hash) *HashType {
	newType := &HashType{
		Name:    name,
		Alias:   alias,
		Width:   width,
		NewFunc: newFunc,
	}

	name2hash[name] = newType
	alias2hash[alias] = newType
	Supported = append(Supported, newType)
	return newType
}

var (
	// MD5 indicates MD5 support
	MD5 = RegisterHash("md5", "MD5", 32, md5.New)

	// SHA1 indicates SHA-1 support
	SHA1 = RegisterHash("sha1", "SHA-1", 40, sha1.New)

	// SHA256 indicates SHA-256 support
	SHA256 = RegisterHash("sha256", "SHA-256", 64, sha256.New)
)

// HashData get hash of one hashType
func HashData(hashType *HashType, data []byte, params ...any) string {
	h := hashType.NewFunc(params...)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// HashReader get hash of one hashType from a reader
func HashReader(hashType *HashType, reader io.Reader, params ...any) (string, error) {
	h := hashType.NewFunc(params...)
	_, err := CopyWithBuffer(h, reader)
	if err != nil {
		return "", errs.NewErr(err, "HashReader error")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// HashFile get hash of one hashType from a model.File
func HashFile(hashType *HashType, file io.ReadSeeker, params ...any) (string, error) {
	str, err := HashReader(hashType, file, params...)
	if err != nil {
		return "", err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return str, err
	}
	return str, nil
}

// fromTypes will return hashers for all the requested types.
func fromTypes(types []*HashType) map[*HashType]hash.Hash {
	hashers := map[*HashType]hash.Hash{}
	for _, t := range types {
		hashers[t] = t.NewFunc()
	}
	return hashers
}

// toMultiWriter will return a set of hashers into a
// single multiwriter, where one write will update all
// the hashers.
func toMultiWriter(h map[*HashType]hash.Hash) io.Writer {
	// Convert to to slice
	var w = make([]io.Writer, 0, len(h))
	for _, v := range h {
		w = append(w, v)
	}
	return io.MultiWriter(w...)
}

// A MultiHasher will construct various hashes on all incoming writes.
type MultiHasher struct {
	w    io.Writer
	size int64
	h    map[*HashType]hash.Hash // Hashes
}

// NewMultiHasher will return a hash writer that will write
// the requested hash types.
func NewMultiHasher(types []*HashType) *MultiHasher {
	hashers := fromTypes(types)
	m := MultiHasher{h: hashers, w: toMultiWriter(hashers)}
	return &m
}

func (m *MultiHasher) Write(p []byte) (n int, err error) {
	n, err = m.w.Write(p)
	m.size += int64(n)
	return n, err
}

func (m *MultiHasher) GetHashInfo() *HashInfo {
	dst := make(map[*HashType]string)
	for k, v := range m.h {
		dst[k] = hex.EncodeToString(v.Sum(nil))
	}
	return &HashInfo{h: dst}
}

// Sum returns the specified hash from the multihasher
func (m *MultiHasher) Sum(hashType *HashType) ([]byte, error) {
	h, ok := m.h[hashType]
	if !ok {
		return nil, ErrUnsupported
	}
	return h.Sum(nil), nil
}

// Size returns the number of bytes written
func (m *MultiHasher) Size() int64 {
	return m.size
}

// A HashInfo contains hash string for one or more hashType
type HashInfo struct {
	h map[*HashType]string `json:"hashInfo"`
}

func NewHashInfoByMap(h map[*HashType]string) HashInfo {
	return HashInfo{h}
}

func NewHashInfo(ht *HashType, str string) HashInfo {
	m := make(map[*HashType]string)
	if ht != nil {
		m[ht] = str
	}
	return HashInfo{h: m}
}

func (hi HashInfo) String() string {
	result, err := json.Marshal(hi.h)
	if err != nil {
		return ""
	}
	return string(result)
}
func FromString(str string) HashInfo {
	hi := NewHashInfo(nil, "")
	var tmp map[string]string
	err := json.Unmarshal([]byte(str), &tmp)
	if err != nil {
		log.Warnf("failed to unmarsh HashInfo from string=%s", str)
	} else {
		for k, v := range tmp {
			if name2hash[k] != nil && len(v) > 0 {
				hi.h[name2hash[k]] = v
			}
		}
	}

	return hi
}
func (hi HashInfo) GetHash(ht *HashType) string {
	return hi.h[ht]
}

func (hi HashInfo) Export() map[*HashType]string {
	return hi.h
}

func (hi HashInfo) All() iter.Seq2[*HashType, string] {
	return func(yield func(*HashType, string) bool) {
		for hashType, hashValue := range hi.h {
			if !yield(hashType, hashValue) {
				return
			}
		}
	}
}
