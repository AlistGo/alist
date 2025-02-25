package quqi

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	stdpath "path"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
	"github.com/minio/sio"
)

// do others that not defined in Driver interface
func (d *Quqi) request(host string, path string, method string, callback base.ReqCallback, resp interface{}) (*resty.Response, error) {
	var (
		reqUrl = url.URL{
			Scheme: "https",
			Host:   "quqi.com",
			Path:   path,
		}
		req    = base.RestyClient.R()
		result BaseRes
	)

	if host != "" {
		reqUrl.Host = host
	}
	req.SetHeaders(map[string]string{
		"Origin": "https://quqi.com",
		"Cookie": d.Cookie,
	})

	if d.GroupID != "" {
		req.SetQueryParam("quqiid", d.GroupID)
	}

	if callback != nil {
		callback(req)
	}

	res, err := req.Execute(method, reqUrl.String())
	if err != nil {
		return nil, err
	}
	// resty.Request.SetResult cannot parse result correctly sometimes
	err = utils.Json.Unmarshal(res.Body(), &result)
	if err != nil {
		return nil, err
	}
	if result.Code != 0 {
		return nil, errors.New(result.Message)
	}
	if resp != nil {
		err = utils.Json.Unmarshal(res.Body(), resp)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (d *Quqi) login() error {
	if d.Addition.Cookie != "" {
		d.Cookie = d.Addition.Cookie
	}
	if d.checkLogin() {
		return nil
	}
	if d.Cookie != "" {
		return errors.New("cookie is invalid")
	}
	if d.Phone == "" {
		return errors.New("phone number is empty")
	}
	if d.Password == "" {
		return errs.EmptyPassword
	}

	resp, err := d.request("", "/auth/person/v2/login/password", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"phone":    d.Phone,
			"password": base64.StdEncoding.EncodeToString([]byte(d.Password)),
		})
	}, nil)
	if err != nil {
		return err
	}

	var cookies []string
	for _, cookie := range resp.RawResponse.Cookies() {
		cookies = append(cookies, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
	}
	d.Cookie = strings.Join(cookies, ";")

	return nil
}

func (d *Quqi) checkLogin() bool {
	if _, err := d.request("", "/auth/account/baseInfo", resty.MethodGet, nil, nil); err != nil {
		return false
	}
	return true
}

// rawExt 保留扩展名大小写
func rawExt(name string) string {
	ext := stdpath.Ext(name)
	if strings.HasPrefix(ext, ".") {
		ext = ext[1:]
	}

	return ext
}

// decryptKey 获取密码
func decryptKey(encodeKey string) []byte {
	// 移除非法字符
	u := strings.ReplaceAll(encodeKey, "[^A-Za-z0-9+\\/]", "")

	// 计算输出字节数组的长度
	o := len(u)
	a := 32

	// 创建输出字节数组
	c := make([]byte, a)

	// 编码循环
	s := uint32(0) // 累加器
	f := 0         // 输出数组索引
	for l := 0; l < o; l++ {
		r := l & 3 // 取模4，得到当前字符在四字节块中的位置
		i := u[l]  // 当前字符的ASCII码

		// 编码当前字符
		switch {
		case i >= 65 && i < 91: // 大写字母
			s |= uint32(i-65) << uint32(6*(3-r))
		case i >= 97 && i < 123: // 小写字母
			s |= uint32(i-71) << uint32(6*(3-r))
		case i >= 48 && i < 58: // 数字
			s |= uint32(i+4) << uint32(6*(3-r))
		case i == 43: // 加号
			s |= uint32(62) << uint32(6*(3-r))
		case i == 47: // 斜杠
			s |= uint32(63) << uint32(6*(3-r))
		}

		// 如果累加器已经包含了四个字符，或者是最后一个字符，则写入输出数组
		if r == 3 || l == o-1 {
			for e := 0; e < 3 && f < a; e, f = e+1, f+1 {
				c[f] = byte(s >> (16 >> e & 24) & 255)
			}
			s = 0
		}
	}

	return c
}

func (d *Quqi) linkFromPreview(id string) (*model.Link, error) {
	var getDocResp GetDocRes
	if _, err := d.request("", "/api/doc/getDoc", resty.MethodPost, func(req *resty.Request) {
		req.SetFormData(map[string]string{
			"quqi_id":   d.GroupID,
			"tree_id":   "1",
			"node_id":   id,
			"client_id": d.ClientID,
		})
	}, &getDocResp); err != nil {
		return nil, err
	}
	if getDocResp.Data.OriginPath == "" {
		return nil, errors.New("cannot get link from preview")
	}
	return &model.Link{
		URL: getDocResp.Data.OriginPath,
		Header: http.Header{
			"Origin": []string{"https://quqi.com"},
			"Cookie": []string{d.Cookie},
		},
	}, nil
}

func (d *Quqi) linkFromDownload(id string) (*model.Link, error) {
	var getDownloadResp GetDownloadResp
	if _, err := d.request("", "/api/doc/getDownload", resty.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(map[string]string{
			"quqi_id":     d.GroupID,
			"tree_id":     "1",
			"node_id":     id,
			"url_type":    "undefined",
			"entry_type":  "undefined",
			"client_id":   d.ClientID,
			"no_redirect": "1",
		})
	}, &getDownloadResp); err != nil {
		return nil, err
	}
	if getDownloadResp.Data.Url == "" {
		return nil, errors.New("cannot get link from download")
	}

	return &model.Link{
		URL: getDownloadResp.Data.Url,
		Header: http.Header{
			"Origin": []string{"https://quqi.com"},
			"Cookie": []string{d.Cookie},
		},
	}, nil
}

func (d *Quqi) linkFromCDN(id string) (*model.Link, error) {
	downloadLink, err := d.linkFromDownload(id)
	if err != nil {
		return nil, err
	}

	var urlExchangeResp UrlExchangeResp
	if _, err = d.request("api.quqi.com", "/preview/downloadInfo/url/exchange", resty.MethodGet, func(req *resty.Request) {
		req.SetQueryParam("url", downloadLink.URL)
	}, &urlExchangeResp); err != nil {
		return nil, err
	}
	if urlExchangeResp.Data.Url == "" {
		return nil, errors.New("cannot get link from cdn")
	}

	// 假设存在未加密的情况
	if !urlExchangeResp.Data.IsEncrypted {
		return &model.Link{
			URL: urlExchangeResp.Data.Url,
			Header: http.Header{
				"Origin": []string{"https://quqi.com"},
				"Cookie": []string{d.Cookie},
			},
		}, nil
	}

	// 根据sio(https://github.com/minio/sio/blob/master/DARE.md)描述及实际测试，得出以下结论：
	// 1. 加密后大小(encrypted_size)-原始文件大小(size) = 加密包的头大小+身份验证标识 = (16+16) * N  ->  N为加密包的数量
	// 2. 原始文件大小(size)+64*1024-1 / (64*1024) = N  ->  每个包的有效负载为64K
	remoteClosers := utils.EmptyClosers()
	payloadSize := int64(1 << 16)
	expiration := time.Until(time.Unix(urlExchangeResp.Data.ExpiredTime, 0))
	resultRangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
		encryptedOffset := httpRange.Start / payloadSize * (payloadSize + 32)
		decryptedOffset := httpRange.Start % payloadSize
		encryptedLength := (httpRange.Length+httpRange.Start+payloadSize-1)/payloadSize*(payloadSize+32) - encryptedOffset
		if httpRange.Length < 0 {
			encryptedLength = httpRange.Length
		} else {
			if httpRange.Length+httpRange.Start >= urlExchangeResp.Data.Size || encryptedLength+encryptedOffset >= urlExchangeResp.Data.EncryptedSize {
				encryptedLength = -1
			}
		}
		//log.Debugf("size: %d\tencrypted_size: %d", urlExchangeResp.Data.Size, urlExchangeResp.Data.EncryptedSize)
		//log.Debugf("http range offset: %d, length: %d", httpRange.Start, httpRange.Length)
		//log.Debugf("encrypted offset: %d, length: %d, decrypted offset: %d", encryptedOffset, encryptedLength, decryptedOffset)

		rrc, err := stream.GetRangeReadCloserFromLink(urlExchangeResp.Data.EncryptedSize, &model.Link{
			URL: urlExchangeResp.Data.Url,
			Header: http.Header{
				"Origin": []string{"https://quqi.com"},
				"Cookie": []string{d.Cookie},
			},
		})
		if err != nil {
			return nil, err
		}

		rc, err := rrc.RangeRead(ctx, http_range.Range{Start: encryptedOffset, Length: encryptedLength})
		remoteClosers.AddClosers(rrc.GetClosers())
		if err != nil {
			return nil, err
		}

		decryptReader, err := sio.DecryptReader(rc, sio.Config{
			MinVersion:     sio.Version10,
			MaxVersion:     sio.Version20,
			CipherSuites:   []byte{sio.CHACHA20_POLY1305, sio.AES_256_GCM},
			Key:            decryptKey(urlExchangeResp.Data.EncryptedKey),
			SequenceNumber: uint32(httpRange.Start / payloadSize),
		})
		if err != nil {
			return nil, err
		}
		bufferReader := bufio.NewReader(decryptReader)
		bufferReader.Discard(int(decryptedOffset))

		return io.NopCloser(bufferReader), nil
	}

	return &model.Link{
		RangeReadCloser: &model.RangeReadCloser{RangeReader: resultRangeReader, Closers: remoteClosers},
		Expiration:      &expiration,
	}, nil
}
