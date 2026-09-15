package quark

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
)

func newUCDeviceID() string {
	// 18 random bytes produce a printable, 24-byte UTDID.
	var b [18]byte
	rand.Read(b[:])
	return base64.StdEncoding.EncodeToString(b[:])
}

// ucDownloadToken implements UC's device token format documented at
// https://github.com/AlistGo/alist/issues/8030#issuecomment-5522861115.
// The fixed key and IV are protocol constants, not storage credentials.
func ucDownloadToken(utdid string) (string, error) {
	if len(utdid) != 24 {
		return "", errors.New("UC UTDID must be 24 bytes")
	}
	// ASCII of the first 16 hex digits of MD5(C88EAC2B-F07C-4FE2-ABDF-C8F049C46C7B).
	key := []byte("2106f55fd41e800c")
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	padding := aes.BlockSize - len(utdid)%aes.BlockSize
	plaintext := append([]byte(utdid), bytes.Repeat([]byte{byte(padding)}, padding)...)
	token := make([]byte, 2+len(plaintext))
	binary.BigEndian.PutUint16(token, 13801)
	cipher.NewCBCEncrypter(block, key).CryptBlocks(token[2:], plaintext)
	return base64.StdEncoding.EncodeToString(token), nil
}
