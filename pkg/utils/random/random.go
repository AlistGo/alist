package random

import (
	"crypto/rand"
	"math/big"
	mathRand "math/rand"
	"time"

	"github.com/google/uuid"
)

var Rand *mathRand.Rand

const letterBytes = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func String(n int) string {
	b := make([]byte, n)
	letterLen := big.NewInt(int64(len(letterBytes)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, letterLen)
		if err != nil {
			panic(err)
		}
		b[i] = letterBytes[idx.Int64()]
	}
	return string(b)
}

func Token() string {
	return "alist-" + uuid.NewString() + String(64)
}

func RangeInt64(left, right int64) int64 {
	return mathRand.Int63n(left+right) - left
}

func init() {
	s := mathRand.NewSource(time.Now().UnixNano())
	Rand = mathRand.New(s)
}
