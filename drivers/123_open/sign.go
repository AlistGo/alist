package _123Open

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"net/url"
	"time"
)

func SignURL(originURL, privateKey string, uid uint64, validDuration time.Duration) (string, error) {
	if privateKey == "" {
		return originURL, nil
	}
	parsed, err := url.Parse(originURL)
	if err != nil {
		return "", err
	}
	ts := time.Now().Add(validDuration).Unix()
	randInt := rand.Int()
	signature := fmt.Sprintf("%d-%d-%d-%x", ts, randInt, uid, md5.Sum([]byte(fmt.Sprintf("%s-%d-%d-%d-%s",
		parsed.Path, ts, randInt, uid, privateKey))))
	query := parsed.Query()
	query.Add("auth_key", signature)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
