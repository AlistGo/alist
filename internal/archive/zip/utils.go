package zip

import (
	"bytes"
	"io"
	"io/fs"
	stdpath "path"
	"strings"

	"github.com/alist-org/alist/v3/internal/archive/tool"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/saintfish/chardet"
	"github.com/yeka/zip"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

type WrapReader struct {
	Reader *zip.Reader
}

func (r *WrapReader) Files() []tool.SubFile {
	ret := make([]tool.SubFile, 0, len(r.Reader.File))
	for _, f := range r.Reader.File {
		ret = append(ret, &WrapFile{f: f})
	}
	return ret
}

type WrapFileInfo struct {
	fs.FileInfo
}

func (f *WrapFileInfo) Name() string {
	return decodeName(f.FileInfo.Name())
}

type WrapFile struct {
	f *zip.File
}

func (f *WrapFile) Name() string {
	return decodeName(f.f.Name)
}

func (f *WrapFile) FileInfo() fs.FileInfo {
	return &WrapFileInfo{FileInfo: f.f.FileInfo()}
}

func (f *WrapFile) Open() (io.ReadCloser, error) {
	return f.f.Open()
}

func (f *WrapFile) IsEncrypted() bool {
	return f.f.IsEncrypted()
}

func (f *WrapFile) SetPassword(password string) {
	f.f.SetPassword(password)
}

func getReader(ss []*stream.SeekableStream) (*zip.Reader, error) {
	if len(ss) > 1 && stdpath.Ext(ss[1].GetName()) == ".z01" {
		// FIXME: Incorrect parsing method for standard multipart zip format
		ss = append(ss[1:], ss[0])
	}
	reader, err := stream.NewMultiReaderAt(ss)
	if err != nil {
		return nil, err
	}
	return zip.NewReader(reader, reader.Size())
}

func filterPassword(err error) error {
	if err != nil && strings.Contains(err.Error(), "password") {
		return errs.WrongArchivePassword
	}
	return err
}

func decodeName(name string) string {
	b := []byte(name)
	detector := chardet.NewTextDetector()
	results, err := detector.DetectAll(b)
	if err != nil {
		return name
	}
	var ce, re, enc encoding.Encoding
	for _, r := range results {
		if r.Confidence > 30 {
			ce = getCommonEncoding(r.Charset)
			if ce != nil {
				break
			}
		}
		if re == nil {
			re = getEncoding(r.Charset)
		}
	}
	if ce != nil {
		enc = ce
	} else if re != nil {
		enc = re
	} else {
		return name
	}
	i := bytes.NewReader(b)
	decoder := transform.NewReader(i, enc.NewDecoder())
	content, _ := io.ReadAll(decoder)
	return string(content)
}

func getCommonEncoding(name string) (enc encoding.Encoding) {
	switch name {
	case "UTF-8":
		enc = unicode.UTF8
	case "UTF-16LE":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "Shift_JIS":
		enc = japanese.ShiftJIS
	case "GB-18030":
		enc = simplifiedchinese.GB18030
	case "EUC-KR":
		enc = korean.EUCKR
	case "Big5":
		enc = traditionalchinese.Big5
	default:
		enc = nil
	}
	return
}

func getEncoding(name string) (enc encoding.Encoding) {
	switch name {
	case "UTF-8":
		enc = unicode.UTF8
	case "UTF-16BE":
		enc = unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	case "UTF-16LE":
		enc = unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "UTF-32BE":
		enc = utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM)
	case "UTF-32LE":
		enc = utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM)
	case "ISO-8859-1":
		enc = charmap.ISO8859_1
	case "ISO-8859-2":
		enc = charmap.ISO8859_2
	case "ISO-8859-3":
		enc = charmap.ISO8859_3
	case "ISO-8859-4":
		enc = charmap.ISO8859_4
	case "ISO-8859-5":
		enc = charmap.ISO8859_5
	case "ISO-8859-6":
		enc = charmap.ISO8859_6
	case "ISO-8859-7":
		enc = charmap.ISO8859_7
	case "ISO-8859-8":
		enc = charmap.ISO8859_8
	case "ISO-8859-8-I":
		enc = charmap.ISO8859_8I
	case "ISO-8859-9":
		enc = charmap.ISO8859_9
	case "windows-1251":
		enc = charmap.Windows1251
	case "windows-1256":
		enc = charmap.Windows1256
	case "KOI8-R":
		enc = charmap.KOI8R
	case "Shift_JIS":
		enc = japanese.ShiftJIS
	case "GB-18030":
		enc = simplifiedchinese.GB18030
	case "EUC-JP":
		enc = japanese.EUCJP
	case "EUC-KR":
		enc = korean.EUCKR
	case "Big5":
		enc = traditionalchinese.Big5
	case "ISO-2022-JP":
		enc = japanese.ISO2022JP
	default:
		enc = nil
	}
	return
}
