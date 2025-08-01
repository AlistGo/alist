package halalcloud

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/http_range"
	"github.com/city404/v6-public-rpc-proto/go/v6/common"
	pbPublicUser "github.com/city404/v6-public-rpc-proto/go/v6/user"
	pubUserFile "github.com/city404/v6-public-rpc-proto/go/v6/userfile"
	"github.com/ipfs/go-cid"
	"github.com/rclone/rclone/lib/readers"
	log "github.com/sirupsen/logrus"
	"github.com/zzzhr1990/go-common-entity/userfile"
)

type HalalCloud struct {
	*HalalCommon
	model.Storage
	Addition

	uploadThread int
}

func (d *HalalCloud) Config() driver.Config {
	return config
}

func (d *HalalCloud) GetAddition() driver.Additional {
	return &d.Addition
}

func (d *HalalCloud) Init(ctx context.Context) error {
	d.uploadThread, _ = strconv.Atoi(d.UploadThread)
	if d.uploadThread < 1 || d.uploadThread > 32 {
		d.uploadThread, d.UploadThread = 3, "3"
	}

	if d.HalalCommon == nil {
		d.HalalCommon = &HalalCommon{
			Common: &Common{},
			AuthService: &AuthService{
				appID: func() string {
					if d.Addition.AppID != "" {
						return d.Addition.AppID
					}
					return AppID
				}(),
				appVersion: func() string {
					if d.Addition.AppVersion != "" {
						return d.Addition.AppVersion
					}
					return AppVersion
				}(),
				appSecret: func() string {
					if d.Addition.AppSecret != "" {
						return d.Addition.AppSecret
					}
					return AppSecret
				}(),
				tr: &TokenResp{
					RefreshToken: d.Addition.RefreshToken,
				},
			},
			UserInfo: &UserInfo{},
			refreshTokenFunc: func(token string) error {
				d.Addition.RefreshToken = token
				op.MustSaveDriverStorage(d)
				return nil
			},
		}
	}

	// 防止重复登录
	if d.Addition.RefreshToken == "" || !d.IsLogin() {
		as, err := d.NewAuthServiceWithOauth()
		if err != nil {
			d.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
			return err
		}
		d.HalalCommon.AuthService = as
		d.SetTokenResp(as.tr)
		op.MustSaveDriverStorage(d)
	}
	var err error
	d.HalalCommon.serv, err = d.NewAuthService(d.Addition.RefreshToken)
	if err != nil {
		return err
	}

	return nil
}

func (d *HalalCloud) Drop(ctx context.Context) error {
	return nil
}

func (d *HalalCloud) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return d.getFiles(ctx, dir)
}

func (d *HalalCloud) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	return d.getLink(ctx, file, args)
}

func (d *HalalCloud) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) (model.Obj, error) {
	return d.makeDir(ctx, parentDir, dirName)
}

func (d *HalalCloud) Move(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.move(ctx, srcObj, dstDir)
}

func (d *HalalCloud) Rename(ctx context.Context, srcObj model.Obj, newName string) (model.Obj, error) {
	return d.rename(ctx, srcObj, newName)
}

func (d *HalalCloud) Copy(ctx context.Context, srcObj, dstDir model.Obj) (model.Obj, error) {
	return d.copy(ctx, srcObj, dstDir)
}

func (d *HalalCloud) Remove(ctx context.Context, obj model.Obj) error {
	return d.remove(ctx, obj)
}

func (d *HalalCloud) Put(ctx context.Context, dstDir model.Obj, stream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {
	return d.put(ctx, dstDir, stream, up)
}

func (d *HalalCloud) IsLogin() bool {
	if d.AuthService.tr == nil {
		return false
	}
	serv, err := d.NewAuthService(d.Addition.RefreshToken)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := pbPublicUser.NewPubUserClient(serv.GetGrpcConnection()).Get(ctx, &pbPublicUser.User{
		Identity: "",
	})
	if result == nil || err != nil {
		return false
	}
	d.UserInfo.Identity = result.Identity
	d.UserInfo.CreateTs = result.CreateTs
	d.UserInfo.Name = result.Name
	d.UserInfo.UpdateTs = result.UpdateTs
	return true
}

type HalalCommon struct {
	*Common
	*AuthService     // 登录信息
	*UserInfo        // 用户信息
	refreshTokenFunc func(token string) error
	serv             *AuthService
}

func (d *HalalCloud) SetTokenResp(tr *TokenResp) {
	d.Addition.RefreshToken = tr.RefreshToken
}

func (d *HalalCloud) getFiles(ctx context.Context, dir model.Obj) ([]model.Obj, error) {

	files := make([]model.Obj, 0)
	limit := int64(100)
	token := ""
	client := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection())

	opDir := d.GetCurrentDir(dir)

	for {
		result, err := client.List(ctx, &pubUserFile.FileListRequest{
			Parent: &pubUserFile.File{Path: opDir},
			ListInfo: &common.ScanListRequest{
				Limit: limit,
				Token: token,
			},
		})
		if err != nil {
			return nil, err
		}

		for i := 0; len(result.Files) > i; i++ {
			files = append(files, (*Files)(result.Files[i]))
		}

		if result.ListInfo == nil || result.ListInfo.Token == "" {
			break
		}
		token = result.ListInfo.Token

	}
	return files, nil
}

func (d *HalalCloud) getLink(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {

	client := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection())
	ctx1, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	result, err := client.ParseFileSlice(ctx1, (*pubUserFile.File)(file.(*Files)))
	if err != nil {
		return nil, err
	}
	fileAddrs := []*pubUserFile.SliceDownloadInfo{}
	var addressDuration int64

	nodesNumber := len(result.RawNodes)
	nodesIndex := nodesNumber - 1
	startIndex, endIndex := 0, nodesIndex
	for nodesIndex >= 0 {
		if nodesIndex >= 200 {
			endIndex = 200
		} else {
			endIndex = nodesNumber
		}
		for ; endIndex <= nodesNumber; endIndex += 200 {
			if endIndex == 0 {
				endIndex = 1
			}
			sliceAddress, err := client.GetSliceDownloadAddress(ctx, &pubUserFile.SliceDownloadAddressRequest{
				Identity: result.RawNodes[startIndex:endIndex],
				Version:  1,
			})
			if err != nil {
				return nil, err
			}
			addressDuration = sliceAddress.ExpireAt
			fileAddrs = append(fileAddrs, sliceAddress.Addresses...)
			startIndex = endIndex
			nodesIndex -= 200
		}

	}

	size := result.FileSize
	chunks := getChunkSizes(result.Sizes)
	resultRangeReader := func(ctx context.Context, httpRange http_range.Range) (io.ReadCloser, error) {
		length := httpRange.Length
		if httpRange.Length >= 0 && httpRange.Start+httpRange.Length >= size {
			length = -1
		}
		if err != nil {
			return nil, fmt.Errorf("open download file failed: %w", err)
		}
		oo := &openObject{
			ctx:     ctx,
			d:       fileAddrs,
			chunk:   &[]byte{},
			chunks:  &chunks,
			skip:    httpRange.Start,
			sha:     result.Sha1,
			shaTemp: sha1.New(),
		}

		return readers.NewLimitedReadCloser(oo, length), nil
	}

	var duration time.Duration
	if addressDuration != 0 {
		duration = time.Until(time.UnixMilli(addressDuration))
	} else {
		duration = time.Until(time.Now().Add(time.Hour))
	}

	resultRangeReadCloser := &model.RangeReadCloser{RangeReader: resultRangeReader}
	return &model.Link{
		RangeReadCloser: resultRangeReadCloser,
		Expiration:      &duration,
	}, nil
}

func (d *HalalCloud) makeDir(ctx context.Context, dir model.Obj, name string) (model.Obj, error) {
	newDir := userfile.NewFormattedPath(d.GetCurrentOpDir(dir, []string{name}, 0)).GetPath()
	_, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).Create(ctx, &pubUserFile.File{
		Path: newDir,
	})
	return nil, err
}

func (d *HalalCloud) move(ctx context.Context, obj model.Obj, dir model.Obj) (model.Obj, error) {
	oldDir := userfile.NewFormattedPath(d.GetCurrentDir(obj)).GetPath()
	newDir := userfile.NewFormattedPath(d.GetCurrentDir(dir)).GetPath()
	_, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).Move(ctx, &pubUserFile.BatchOperationRequest{
		Source: []*pubUserFile.File{
			{
				Identity: obj.GetID(),
				Path:     oldDir,
			},
		},
		Dest: &pubUserFile.File{
			Identity: dir.GetID(),
			Path:     newDir,
		},
	})
	return nil, err
}

func (d *HalalCloud) rename(ctx context.Context, obj model.Obj, name string) (model.Obj, error) {
	id := obj.GetID()
	newPath := userfile.NewFormattedPath(d.GetCurrentOpDir(obj, []string{name}, 0)).GetPath()

	_, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).Rename(ctx, &pubUserFile.File{
		Path:     newPath,
		Identity: id,
		Name:     name,
	})
	return nil, err
}

func (d *HalalCloud) copy(ctx context.Context, obj model.Obj, dir model.Obj) (model.Obj, error) {
	id := obj.GetID()
	sourcePath := userfile.NewFormattedPath(d.GetCurrentDir(obj)).GetPath()
	if len(id) > 0 {
		sourcePath = ""
	}
	dest := &pubUserFile.File{
		Identity: dir.GetID(),
		Path:     userfile.NewFormattedPath(d.GetCurrentDir(dir)).GetPath(),
	}
	_, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).Copy(ctx, &pubUserFile.BatchOperationRequest{
		Source: []*pubUserFile.File{
			{
				Path:     sourcePath,
				Identity: id,
			},
		},
		Dest: dest,
	})
	return nil, err
}

func (d *HalalCloud) remove(ctx context.Context, obj model.Obj) error {
	id := obj.GetID()
	newPath := userfile.NewFormattedPath(d.GetCurrentDir(obj)).GetPath()
	//if len(id) > 0 {
	//	newPath = ""
	//}
	_, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).Delete(ctx, &pubUserFile.BatchOperationRequest{
		Source: []*pubUserFile.File{
			{
				Path:     newPath,
				Identity: id,
			},
		},
	})
	return err
}

func (d *HalalCloud) put(ctx context.Context, dstDir model.Obj, fileStream model.FileStreamer, up driver.UpdateProgress) (model.Obj, error) {

	newDir := path.Join(dstDir.GetPath(), fileStream.GetName())

	// https://github.com/city404/v6-public-rpc-proto/wiki/0.100.000-%E6%96%87%E4%BB%B6%E4%B8%8A%E4%BC%A0

	// https://github.com/halalcloud/golang-sdk/blob/652cd8d99c8329b6a975b608d094944cb006d757/cmd/disk/upload.go#L73
	result, err := pubUserFile.NewPubUserFileClient(d.HalalCommon.serv.GetGrpcConnection()).CreateUploadTask(ctx, &pubUserFile.File{
		// Parent: &pubUserFile.File{Path: currentDir},
		Path: newDir,
		//ContentIdentity: args[1],
		Size: fileStream.GetSize(),
	})
	if err != nil {
		return nil, err
	}
	if result.Created {
		return nil, fmt.Errorf("upload file has been created")
	}
	log.Debugf("[halalcloud] Upload task started, total size: %d, block size: %d -> %s\n", fileStream.GetSize(), result.BlockSize, result.Task)
	slicesCount := int(math.Ceil(float64(fileStream.GetSize()) / float64(result.BlockSize)))
	bufferSize := int(result.BlockSize)
	buffer := make([]byte, bufferSize)
	slicesList := make([]string, 0)
	codec := uint64(0x55)
	if result.BlockCodec > 0 {
		codec = uint64(result.BlockCodec)
	}
	mhType := uint64(0x12)
	if result.BlockHashType > 0 {
		mhType = uint64(result.BlockHashType)
	}
	prefix := cid.Prefix{
		Codec:    codec,
		MhLength: -1,
		MhType:   mhType,
		Version:  1,
	}
	// read file
	reader := driver.NewLimitedUploadStream(ctx, fileStream)
	for {
		n, err := io.ReadFull(reader, buffer)
		if n > 0 {
			data := buffer[:n]
			uploadCid, err := postFileSlice(data, result.Task, result.UploadAddress, prefix)
			if err != nil {
				return nil, err
			}
			slicesList = append(slicesList, uploadCid.String())
			up(float64(len(slicesList)) * 90 / float64(slicesCount))
		}
		if err == io.EOF || n == 0 {
			break
		}
	}
	up(95.0)
	newFile, err := makeFile(slicesList, result.Task, result.UploadAddress)
	if err != nil {
		return nil, err
	}
	log.Debugf("[halalcloud] File uploaded, cid: %s\n", newFile.ContentIdentity)
	return nil, err

}

func makeFile(fileSlice []string, taskID string, uploadAddress string) (*pubUserFile.File, error) {
	accessUrl := uploadAddress + "/" + taskID
	u, err := url.Parse(accessUrl)
	if err != nil {
		return nil, err
	}
	n, _ := json.Marshal(fileSlice)
	httpRequest := http.Request{
		Method: http.MethodPost,
		URL:    u,
		Header: map[string][]string{
			"Accept":       {"application/json"},
			"Content-Type": {"application/json"},
			//"Content-Length": {fmt.Sprintf("%d", len(n))},
		},
		Body: io.NopCloser(bytes.NewReader(n)),
	}
	httpResponse, err := base.HttpClient.Do(&httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK && httpResponse.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(httpResponse.Body)
		fmt.Println(string(b))
		return nil, fmt.Errorf("mk file slice failed, status code: %d", httpResponse.StatusCode)
	}
	b, _ := io.ReadAll(httpResponse.Body)
	var result *pubUserFile.File
	err = json.Unmarshal(b, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func postFileSlice(fileSlice []byte, taskID string, uploadAddress string, preix cid.Prefix) (cid.Cid, error) {
	// 1. sum file slice
	newCid, err := preix.Sum(fileSlice)
	if err != nil {
		return cid.Undef, err
	}
	// 2. post file slice
	sliceCidString := newCid.String()
	// /{taskID}/{sliceID}
	accessUrl := uploadAddress + "/" + taskID + "/" + sliceCidString
	// get {accessUrl} in {getTimeOut}
	u, err := url.Parse(accessUrl)
	if err != nil {
		return cid.Undef, err
	}
	// header: accept: application/json
	// header: content-type: application/octet-stream
	// header: content-length: {fileSlice.length}
	// header: x-content-cid: {sliceCidString}
	// header: x-task-id: {taskID}
	httpRequest := http.Request{
		Method: http.MethodGet,
		URL:    u,
		Header: map[string][]string{
			"Accept": {"application/json"},
		},
	}
	httpResponse, err := base.HttpClient.Do(&httpRequest)
	if err != nil {
		return cid.Undef, err
	}
	if httpResponse.StatusCode != http.StatusOK {
		return cid.Undef, fmt.Errorf("check file slice failed, status code: %d", httpResponse.StatusCode)
	}
	var result bool
	b, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return cid.Undef, err
	}
	err = json.Unmarshal(b, &result)
	if err != nil {
		return cid.Undef, err
	}
	if result {
		log.Debugf("[halalcloud] Slice exists, cid: %s\n", newCid)
		return newCid, nil
	}

	httpRequest = http.Request{
		Method: http.MethodPost,
		URL:    u,
		Header: map[string][]string{
			"Accept":       {"application/json"},
			"Content-Type": {"application/octet-stream"},
			// "Content-Length": {fmt.Sprintf("%d", len(fileSlice))},
		},
		Body: io.NopCloser(bytes.NewReader(fileSlice)),
	}
	httpResponse, err = base.HttpClient.Do(&httpRequest)
	if err != nil {
		return cid.Undef, err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK && httpResponse.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(httpResponse.Body)
		fmt.Println(string(b))
		return cid.Undef, fmt.Errorf("upload file slice failed, status code: %d", httpResponse.StatusCode)
	}
	log.Debugf("[halalcloud] Slice uploaded, cid: %s\n", newCid)
	return newCid, nil
}

var _ driver.Driver = (*HalalCloud)(nil)
