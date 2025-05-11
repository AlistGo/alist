package utils

import (
	"strconv"
)

// ParseInt 将字符串解析为int64
func ParseInt(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
