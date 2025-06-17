package baiduphoto

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-resty/resty/v2"
)

const (
	API_URL         = "https://photo.baidu.com/youai"
	USER_API_URL    = API_URL + "/user/v1"
	ALBUM_API_URL   = API_URL + "/album/v1"
	FILE_API_URL_V1 = API_URL + "/file/v1"
	FILE_API_URL_V2 = API_URL + "/file/v2"
)

func (d *BaiduPhoto) Request(client *resty.Client, furl string, method string, callback base.ReqCallback, resp interface{}) (*resty.Response, error) {
	req := client.R().
		// SetQueryParam("access_token", d.AccessToken)
		SetHeader("Cookie", d.Cookie)
	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	res, err := req.Execute(method, furl)
	if err != nil {
		return nil, err
	}

	erron := utils.Json.Get(res.Body(), "errno").ToInt()
	switch erron {
	case 0:
		break
	case 50805:
		return nil, fmt.Errorf("you have joined album")
	case 50820:
		return nil, fmt.Errorf("no shared albums found")
	case 50100:
		return nil, fmt.Errorf("illegal title, only supports 50 characters")
	// case -6:
	// 	if err = d.refreshToken(); err != nil {
	// 		return nil, err
	// 	}
	default:
		return nil, fmt.Errorf("errno: %d, refer to https://photo.baidu.com/union/doc", erron)
	}
	return res, nil
}

//func (d *BaiduPhoto) Request(furl string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
//	res, err := d.request(furl, method, callback, resp)
//	if err != nil {
//		return nil, err
//	}
//	return res.Body(), nil
//}

// func (d *BaiduPhoto) refreshToken() error {
// 	u := "https://openapi.baidu.com/oauth/2.0/token"
// 	var resp base.TokenResp
// 	var e TokenErrResp
// 	_, err := base.RestyClient.R().SetResult(&resp).SetError(&e).SetQueryParams(map[string]string{
// 		"grant_type":    "refresh_token",
// 		"refresh_token": d.RefreshToken,
// 		"client_id":     d.ClientID,
// 		"client_secret": d.ClientSecret,
// 	}).Get(u)
// 	if err != nil {
// 		return err
// 	}
// 	if e.ErrorMsg != "" {
// 		return &e
// 	}
// 	if resp.RefreshToken == "" {
// 		return errs.EmptyToken
// 	}
// 	d.AccessToken, d.RefreshToken = resp.AccessToken, resp.RefreshToken
// 	op.MustSaveDriverStorage(d)
// 	return nil
// }

func (d *BaiduPhoto) Get(furl string, callback base.ReqCallback, resp interface{}) (*resty.Response, error) {
	return d.Request(base.RestyClient, furl, http.MethodGet, callback, resp)
}

func (d *BaiduPhoto) Post(furl string, callback base.ReqCallback, resp interface{}) (*resty.Response, error) {
	return d.Request(base.RestyClient, furl, http.MethodPost, callback, resp)
}

// 获取所有文件
func (d *BaiduPhoto) GetAllFile(ctx context.Context) (files []File, err error) {
	var cursor string
	for {
		var resp FileListResp
		_, err = d.Get(FILE_API_URL_V1+"/list", func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(map[string]string{
				"need_thumbnail":     "1",
				"need_filter_hidden": "0",
				"cursor":             cursor,
			})
		}, &resp)
		if err != nil {
			return
		}

		files = append(files, resp.List...)
		if !resp.HasNextPage() {
			return
		}
		cursor = resp.Cursor
	}
}

// 删除根文件
func (d *BaiduPhoto) DeleteFile(ctx context.Context, file *File) error {
	_, err := d.Get(FILE_API_URL_V1+"/delete", func(req *resty.Request) {
		req.SetContext(ctx)
		req.SetQueryParams(map[string]string{
			"fsid_list": fmt.Sprintf("[%d]", file.Fsid),
		})
	}, nil)
	return err
}

// 获取所有相册
func (d *BaiduPhoto) GetAllAlbum(ctx context.Context) (albums []Album, err error) {
	var cursor string
	for {
		var resp AlbumListResp
		_, err = d.Get(ALBUM_API_URL+"/list", func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(map[string]string{
				"need_amount": "1",
				"limit":       "100",
				"cursor":      cursor,
			})
		}, &resp)
		if err != nil {
			return
		}
		if albums == nil {
			albums = make([]Album, 0, resp.TotalCount)
		}

		cursor = resp.Cursor
		albums = append(albums, resp.List...)

		if !resp.HasNextPage() {
			return
		}
	}
}

// 获取相册中所有文件
func (d *BaiduPhoto) GetAllAlbumFile(ctx context.Context, album *Album, passwd string) (files []AlbumFile, err error) {
	var cursor string
	for {
		var resp AlbumFileListResp
		_, err = d.Get(ALBUM_API_URL+"/listfile", func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(map[string]string{
				"album_id":    album.AlbumID,
				"need_amount": "1",
				"limit":       "1000",
				"passwd":      passwd,
				"cursor":      cursor,
			})
		}, &resp)
		if err != nil {
			return
		}
		if files == nil {
			files = make([]AlbumFile, 0, resp.TotalCount)
		}

		cursor = resp.Cursor
		files = append(files, resp.List...)

		if !resp.HasNextPage() {
			return
		}
	}
}

// 创建相册
func (d *BaiduPhoto) CreateAlbum(ctx context.Context, name string) (*Album, error) {
	var resp JoinOrCreateAlbumResp
	_, err := d.Post(ALBUM_API_URL+"/create", func(r *resty.Request) {
		r.SetContext(ctx).SetResult(&resp)
		r.SetQueryParams(map[string]string{
			"title":  name,
			"tid":    getTid(),
			"source": "0",
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	return d.GetAlbumDetail(ctx, resp.AlbumID)
}

// 相册改名
func (d *BaiduPhoto) SetAlbumName(ctx context.Context, album *Album, name string) (*Album, error) {
	_, err := d.Post(ALBUM_API_URL+"/settitle", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetFormData(map[string]string{
			"title":    name,
			"album_id": album.AlbumID,
			"tid":      fmt.Sprint(album.Tid),
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	return renameAlbum(album, name), nil
}

// 删除相册
func (d *BaiduPhoto) DeleteAlbum(ctx context.Context, album *Album) error {
	_, err := d.Post(ALBUM_API_URL+"/delete", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetFormData(map[string]string{
			"album_id":            album.AlbumID,
			"tid":                 fmt.Sprint(album.Tid),
			"delete_origin_image": BoolToIntStr(d.DeleteOrigin), // 是否删除原图 0 不删除 1 删除
		})
	}, nil)
	return err
}

// 删除相册文件
func (d *BaiduPhoto) DeleteAlbumFile(ctx context.Context, file *AlbumFile) error {
	_, err := d.Post(ALBUM_API_URL+"/delfile", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetFormData(map[string]string{
			"album_id":   fmt.Sprint(file.AlbumID),
			"tid":        fmt.Sprint(file.Tid),
			"list":       fmt.Sprintf(`[{"fsid":%d,"uk":%d}]`, file.Fsid, file.Uk),
			"del_origin": BoolToIntStr(d.DeleteOrigin), // 是否删除原图 0 不删除 1 删除
		})
	}, nil)
	return err
}

// 增加相册文件
func (d *BaiduPhoto) AddAlbumFile(ctx context.Context, album *Album, file *File) (*AlbumFile, error) {
	_, err := d.Get(ALBUM_API_URL+"/addfile", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetQueryParams(map[string]string{
			"album_id": fmt.Sprint(album.AlbumID),
			"tid":      fmt.Sprint(album.Tid),
			"list":     fsidsFormatNotUk(file.Fsid),
		})
	}, nil)
	if err != nil {
		return nil, err
	}
	return moveFileToAlbumFile(file, album, d.Uk), nil
}

// 保存相册文件为根文件
func (d *BaiduPhoto) CopyAlbumFile(ctx context.Context, file *AlbumFile) (*File, error) {
	var resp CopyFileResp
	_, err := d.Post(ALBUM_API_URL+"/copyfile", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetFormData(map[string]string{
			"album_id": file.AlbumID,
			"tid":      fmt.Sprint(file.Tid),
			"uk":       fmt.Sprint(file.Uk),
			"list":     fsidsFormatNotUk(file.Fsid),
		})
		r.SetResult(&resp)
	}, nil)
	if err != nil {
		return nil, err
	}
	return copyFile(file, &resp.List[0]), nil
}

// 加入相册
func (d *BaiduPhoto) JoinAlbum(ctx context.Context, code string) (*Album, error) {
	var resp InviteResp
	_, err := d.Get(ALBUM_API_URL+"/querypcode", func(req *resty.Request) {
		req.SetContext(ctx)
		req.SetQueryParams(map[string]string{
			"pcode": code,
			"web":   "1",
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	var resp2 JoinOrCreateAlbumResp
	_, err = d.Get(ALBUM_API_URL+"/join", func(req *resty.Request) {
		req.SetContext(ctx)
		req.SetQueryParams(map[string]string{
			"invite_code": resp.Pdata.InviteCode,
		})
	}, &resp2)
	if err != nil {
		return nil, err
	}
	return d.GetAlbumDetail(ctx, resp2.AlbumID)
}

// 获取相册详细信息
func (d *BaiduPhoto) GetAlbumDetail(ctx context.Context, albumID string) (*Album, error) {
	var album Album
	_, err := d.Get(ALBUM_API_URL+"/detail", func(req *resty.Request) {
		req.SetContext(ctx).SetResult(&album)
		req.SetQueryParams(map[string]string{
			"album_id": albumID,
		})
	}, &album)
	if err != nil {
		return nil, err
	}
	return &album, nil
}

func (d *BaiduPhoto) linkAlbum(ctx context.Context, file *AlbumFile, args model.LinkArgs) (*model.Link, error) {
	headers := map[string]string{
		"User-Agent": base.UserAgent,
	}
	if args.Header.Get("User-Agent") != "" {
		headers["User-Agent"] = args.Header.Get("User-Agent")
	}
	if !utils.IsLocalIPAddr(args.IP) {
		headers["X-Forwarded-For"] = args.IP
	}

	resp, err := d.Request(base.NoRedirectClient, ALBUM_API_URL+"/download", http.MethodHead, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetHeaders(headers)
		r.SetQueryParams(map[string]string{
			"fsid":     fmt.Sprint(file.Fsid),
			"album_id": file.AlbumID,
			"tid":      fmt.Sprint(file.Tid),
			"uk":       fmt.Sprint(file.Uk),
		})
	}, nil)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() != 302 {
		return nil, fmt.Errorf("not found 302 redirect")
	}

	location := resp.Header().Get("Location")

	link := &model.Link{
		URL: location,
		Header: http.Header{
			"User-Agent": []string{headers["User-Agent"]},
			"Referer":    []string{"https://photo.baidu.com/"},
		},
	}
	return link, nil
}

func (d *BaiduPhoto) linkFile(ctx context.Context, file *File, args model.LinkArgs) (*model.Link, error) {
	headers := map[string]string{
		"User-Agent": base.UserAgent,
	}
	if args.Header.Get("User-Agent") != "" {
		headers["User-Agent"] = args.Header.Get("User-Agent")
	}
	if !utils.IsLocalIPAddr(args.IP) {
		headers["X-Forwarded-For"] = args.IP
	}

	var downloadUrl struct {
		Dlink string `json:"dlink"`
	}
	_, err := d.Get(FILE_API_URL_V2+"/download", func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetHeaders(headers)
		r.SetQueryParams(map[string]string{
			"fsid": fmt.Sprint(file.Fsid),
		})
	}, &downloadUrl)

	// resp, err := d.Request(base.NoRedirectClient, FILE_API_URL_V1+"/download", http.MethodHead, func(r *resty.Request) {
	// 	r.SetContext(ctx)
	// 	r.SetHeaders(headers)
	// 	r.SetQueryParams(map[string]string{
	// 		"fsid": fmt.Sprint(file.Fsid),
	// 	})
	// }, nil)

	if err != nil {
		return nil, err
	}

	// if resp.StatusCode() != 302 {
	// 	return nil, fmt.Errorf("not found 302 redirect")
	// }

	// location := resp.Header().Get("Location")
	link := &model.Link{
		URL: downloadUrl.Dlink,
		Header: http.Header{
			"User-Agent": []string{headers["User-Agent"]},
			"Referer":    []string{"https://photo.baidu.com/"},
		},
	}
	return link, nil
}

/*func (d *BaiduPhoto) linkStreamAlbum(ctx context.Context, file *AlbumFile) (*model.Link, error) {
	return &model.Link{
		Header: http.Header{},
		Writer: func(w io.Writer) error {
			res, err := d.Get(ALBUM_API_URL+"/streaming", func(r *resty.Request) {
				r.SetContext(ctx)
				r.SetQueryParams(map[string]string{
					"fsid":     fmt.Sprint(file.Fsid),
					"album_id": file.AlbumID,
					"tid":      fmt.Sprint(file.Tid),
					"uk":       fmt.Sprint(file.Uk),
				}).SetDoNotParseResponse(true)
			}, nil)
			if err != nil {
				return err
			}
			defer res.RawBody().Close()
			_, err = io.Copy(w, res.RawBody())
			return err
		},
	}, nil
}*/

/*func (d *BaiduPhoto) linkStream(ctx context.Context, file *File) (*model.Link, error) {
	return &model.Link{
		Header: http.Header{},
		Writer: func(w io.Writer) error {
			res, err := d.Get(FILE_API_URL_V1+"/streaming", func(r *resty.Request) {
				r.SetContext(ctx)
				r.SetQueryParams(map[string]string{
					"fsid": fmt.Sprint(file.Fsid),
				}).SetDoNotParseResponse(true)
			}, nil)
			if err != nil {
				return err
			}
			defer res.RawBody().Close()
			_, err = io.Copy(w, res.RawBody())
			return err
		},
	}, nil
}*/

// 获取uk
func (d *BaiduPhoto) uInfo() (*UInfo, error) {
	var info UInfo
	_, err := d.Get(USER_API_URL+"/getuinfo", func(req *resty.Request) {

	}, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *BaiduPhoto) getBDStoken() (string, error) {
	var info struct {
		Result struct {
			Bdstoken string `json:"bdstoken"`
			Token    string `json:"token"`
			Uk       int64  `json:"uk"`
		} `json:"result"`
	}
	_, err := d.Get("https://pan.baidu.com/api/gettemplatevariable?fields=[%22bdstoken%22,%22token%22,%22uk%22]", nil, &info)
	if err != nil {
		return "", err
	}
	return info.Result.Bdstoken, nil
}

func DecryptMd5(encryptMd5 string) string {
	if _, err := hex.DecodeString(encryptMd5); err == nil {
		return encryptMd5
	}

	var out strings.Builder
	out.Grow(len(encryptMd5))
	for i, n := 0, int64(0); i < len(encryptMd5); i++ {
		if i == 9 {
			n = int64(unicode.ToLower(rune(encryptMd5[i])) - 'g')
		} else {
			n, _ = strconv.ParseInt(encryptMd5[i:i+1], 16, 64)
		}
		out.WriteString(strconv.FormatInt(n^int64(15&i), 16))
	}

	encryptMd5 = out.String()
	return encryptMd5[8:16] + encryptMd5[:8] + encryptMd5[24:32] + encryptMd5[16:24]
}

func EncryptMd5(originalMd5 string) string {
	reversed := originalMd5[8:16] + originalMd5[:8] + originalMd5[24:32] + originalMd5[16:24]

	var out strings.Builder
	out.Grow(len(reversed))
	for i, n := 0, int64(0); i < len(reversed); i++ {
		n, _ = strconv.ParseInt(reversed[i:i+1], 16, 64)
		n ^= int64(15 & i)
		if i == 9 {
			out.WriteRune(rune(n) + 'g')
		} else {
			out.WriteString(strconv.FormatInt(n, 16))
		}
	}
	return out.String()
}
