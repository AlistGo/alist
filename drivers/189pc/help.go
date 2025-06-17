package _189pc

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils/random"
)

func clientSuffix() map[string]string {
	rand := random.Rand
	return map[string]string{
		"clientType": PC,
		"version":    VERSION,
		"channelId":  CHANNEL_ID,
		"rand":       fmt.Sprintf("%d_%d", rand.Int63n(1e5), rand.Int63n(1e10)),
	}
}

// 带params的SignatureOfHmac HMAC签名
func signatureOfHmac(sessionSecret, sessionKey, operate, fullUrl, dateOfGmt, param string) string {
	urlpath := regexp.MustCompile(`://[^/]+((/[^/\s?#]+)*)`).FindStringSubmatch(fullUrl)[1]
	mac := hmac.New(sha1.New, []byte(sessionSecret))
	data := fmt.Sprintf("SessionKey=%s&Operate=%s&RequestURI=%s&Date=%s", sessionKey, operate, urlpath, dateOfGmt)
	if param != "" {
		data += fmt.Sprintf("&params=%s", param)
	}
	mac.Write([]byte(data))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}

// RAS 加密用户名密码
func RsaEncrypt(publicKey, origData string) string {
	block, _ := pem.Decode([]byte(publicKey))
	pubInterface, _ := x509.ParsePKIXPublicKey(block.Bytes)
	data, _ := rsa.EncryptPKCS1v15(rand.Reader, pubInterface.(*rsa.PublicKey), []byte(origData))
	return strings.ToUpper(hex.EncodeToString(data))
}

// aes 加密params
func AesECBEncrypt(data, key string) string {
	block, _ := aes.NewCipher([]byte(key))
	paddingData := PKCS7Padding([]byte(data), block.BlockSize())
	decrypted := make([]byte, len(paddingData))
	size := block.BlockSize()
	for src, dst := paddingData, decrypted; len(src) > 0; src, dst = src[size:], dst[size:] {
		block.Encrypt(dst[:size], src[:size])
	}
	return strings.ToUpper(hex.EncodeToString(decrypted))
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

// 获取http规范的时间
func getHttpDateStr() string {
	return time.Now().UTC().Format(http.TimeFormat)
}

// 时间戳
func timestamp() int64 {
	return time.Now().UTC().UnixNano() / 1e6
}

func MustParseTime(str string) *time.Time {
	lastOpTime, _ := time.ParseInLocation("2006-01-02 15:04:05 -07", str+" +08", time.Local)
	return &lastOpTime
}

type Time time.Time

func (t *Time) UnmarshalJSON(b []byte) error { return t.Unmarshal(b) }
func (t *Time) UnmarshalXML(e *xml.Decoder, ee xml.StartElement) error {
	b, err := e.Token()
	if err != nil {
		return err
	}
	if b, ok := b.(xml.CharData); ok {
		if err = t.Unmarshal(b); err != nil {
			return err
		}
	}
	return e.Skip()
}
func (t *Time) Unmarshal(b []byte) error {
	bs := strings.Trim(string(b), "\"")
	var v time.Time
	var err error
	for _, f := range []string{"2006-01-02 15:04:05 -07", "Jan 2, 2006 15:04:05 PM -07"} {
		v, err = time.ParseInLocation(f, bs+" +08", time.Local)
		if err == nil {
			break
		}
	}
	*t = Time(v)
	return err
}

type String string

func (t *String) UnmarshalJSON(b []byte) error { return t.Unmarshal(b) }
func (t *String) UnmarshalXML(e *xml.Decoder, ee xml.StartElement) error {
	b, err := e.Token()
	if err != nil {
		return err
	}
	if b, ok := b.(xml.CharData); ok {
		if err = t.Unmarshal(b); err != nil {
			return err
		}
	}
	return e.Skip()
}
func (s *String) Unmarshal(b []byte) error {
	*s = String(bytes.Trim(b, "\""))
	return nil
}

func toFamilyOrderBy(o string) string {
	switch o {
	case "filename":
		return "1"
	case "filesize":
		return "2"
	case "lastOpTime":
		return "3"
	default:
		return "1"
	}
}

func toDesc(o string) string {
	switch o {
	case "desc":
		return "true"
	case "asc":
		fallthrough
	default:
		return "false"
	}
}

func ParseHttpHeader(str string) map[string]string {
	header := make(map[string]string)
	for _, value := range strings.Split(str, "&") {
		if k, v, found := strings.Cut(value, "="); found {
			header[k] = v
		}
	}
	return header
}

func MustString(str string, err error) string {
	return str
}

func BoolToNumber(b bool) int {
	if b {
		return 1
	}
	return 0
}

// 计算分片大小
// 对分片数量有限制
// 10MIB 20 MIB 999片
// 50MIB 60MIB 70MIB 80MIB ∞MIB 1999片
func partSize(size int64) int64 {
	const DEFAULT = 1024 * 1024 * 10 // 10MIB
	if size > DEFAULT*2*999 {
		return int64(math.Max(math.Ceil((float64(size)/1999) /*=单个切片大小*/ /float64(DEFAULT)) /*=倍率*/, 5) * DEFAULT)
	}
	if size > DEFAULT*999 {
		return DEFAULT * 2 // 20MIB
	}
	return DEFAULT
}

func isBool(bs ...bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}

func IF[V any](o bool, t V, f V) V {
	if o {
		return t
	}
	return f
}

type WrapFileStreamer struct {
	model.FileStreamer
	Name string
}

func (w *WrapFileStreamer) GetName() string {
	return w.Name
}
