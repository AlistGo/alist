package thunder

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/stream"
	"github.com/alist-org/alist/v3/pkg/utils"
	hash_extend "github.com/alist-org/alist/v3/pkg/utils/hash"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-resty/resty/v2"
)

type Thunder struct {
	*XunLeiCommon
	model.Storage
	Addition

	identity string
}

func (x *Thunder) Config() driver.Config {
	return config
}

func (x *Thunder) GetAddition() driver.Additional {
	return &x.Addition
}

func (x *Thunder) Init(ctx context.Context) (err error) {
	// 初始化所需参数
	if x.XunLeiCommon == nil {
		x.XunLeiCommon = &XunLeiCommon{
			Common: &Common{
				client: base.NewRestyClient(),
				Algorithms: []string{
					"9uJNVj/wLmdwKrJaVj/omlQ",
					"Oz64Lp0GigmChHMf/6TNfxx7O9PyopcczMsnf",
					"Eb+L7Ce+Ej48u",
					"jKY0",
					"ASr0zCl6v8W4aidjPK5KHd1Lq3t+vBFf41dqv5+fnOd",
					"wQlozdg6r1qxh0eRmt3QgNXOvSZO6q/GXK",
					"gmirk+ciAvIgA/cxUUCema47jr/YToixTT+Q6O",
					"5IiCoM9B1/788ntB",
					"P07JH0h6qoM6TSUAK2aL9T5s2QBVeY9JWvalf",
					"+oK0AN",
				},
				DeviceID: func() string {
					if len(x.DeviceID) != 32 {
						return utils.GetMD5EncodeStr(x.DeviceID)
					}
					return x.DeviceID
				}(),
				ClientID:          "Xp6vsxz_7IYVw2BB",
				ClientSecret:      "Xp6vsy4tN9toTVdMSpomVdXpRmES",
				ClientVersion:     "8.31.0.9726",
				PackageName:       "com.xunlei.downloadprovider",
				UserAgent:         "ANDROID-com.xunlei.downloadprovider/8.31.0.9726 netWorkType/5G appid/40 deviceName/Xiaomi_M2004j7ac deviceModel/M2004J7AC OSVersion/12 protocolVersion/301 platformVersion/10 sdkVersion/512000 Oauth2Client/0.9 (Linux 4_14_186-perf-gddfs8vbb238b) (JAVA 0)",
				DownloadUserAgent: "Dalvik/2.1.0 (Linux; U; Android 12; M2004J7AC Build/SP1A.210812.016)",
				refreshCTokenCk: func(token string) {
					x.CaptchaToken = token
					op.MustSaveDriverStorage(x)
				},
			},
			refreshTokenFunc: func() error {
				// 通过RefreshToken刷新
				token, err := x.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					// 重新登录
					token, err = x.Login(x.Username, x.Password)
					if err != nil {
						x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
						op.MustSaveDriverStorage(x)
					}
					// 清空 信任密钥
					x.Addition.CreditKey = ""
				}
				x.SetTokenResp(token)
				return err
			},
		}
	}

	// 自定义验证码token
	ctoekn := strings.TrimSpace(x.CaptchaToken)
	if ctoekn != "" {
		x.SetCaptchaToken(ctoekn)
	}

	if x.Addition.CreditKey != "" {
		x.SetCreditKey(x.Addition.CreditKey)
	}

	if x.Addition.DeviceID != "" {
		x.Common.DeviceID = x.Addition.DeviceID
	} else {
		x.Addition.DeviceID = x.Common.DeviceID
		op.MustSaveDriverStorage(x)
	}

	// 防止重复登录
	identity := x.GetIdentity()
	if x.identity != identity || !x.IsLogin() {
		x.identity = identity
		// 登录
		token, err := x.Login(x.Username, x.Password)
		if err != nil {
			return err
		}
		// 清空 信任密钥
		x.Addition.CreditKey = ""
		x.SetTokenResp(token)
	}
	return nil
}

func (x *Thunder) Drop(ctx context.Context) error {
	return nil
}

type ThunderExpert struct {
	*XunLeiCommon
	model.Storage
	ExpertAddition

	identity string
}

func (x *ThunderExpert) Config() driver.Config {
	return configExpert
}

func (x *ThunderExpert) GetAddition() driver.Additional {
	return &x.ExpertAddition
}

func (x *ThunderExpert) Init(ctx context.Context) (err error) {
	// 防止重复登录
	identity := x.GetIdentity()
	if identity != x.identity || !x.IsLogin() {
		x.identity = identity
		x.XunLeiCommon = &XunLeiCommon{
			Common: &Common{
				client: base.NewRestyClient(),

				DeviceID: func() string {
					if len(x.DeviceID) != 32 {
						return utils.GetMD5EncodeStr(x.DeviceID)
					}
					return x.DeviceID
				}(),
				ClientID:          x.ClientID,
				ClientSecret:      x.ClientSecret,
				ClientVersion:     x.ClientVersion,
				PackageName:       x.PackageName,
				UserAgent:         x.UserAgent,
				DownloadUserAgent: x.DownloadUserAgent,
				UseVideoUrl:       x.UseVideoUrl,

				refreshCTokenCk: func(token string) {
					x.CaptchaToken = token
					op.MustSaveDriverStorage(x)
				},
			},
		}

		if x.CaptchaToken != "" {
			x.SetCaptchaToken(x.CaptchaToken)
		}

		if x.ExpertAddition.CreditKey != "" {
			x.SetCreditKey(x.ExpertAddition.CreditKey)
		}

		if x.ExpertAddition.DeviceID != "" {
			x.Common.DeviceID = x.ExpertAddition.DeviceID
		} else {
			x.ExpertAddition.DeviceID = x.Common.DeviceID
			op.MustSaveDriverStorage(x)
		}

		// 签名方法
		if x.SignType == "captcha_sign" {
			x.Common.Timestamp = x.Timestamp
			x.Common.CaptchaSign = x.CaptchaSign
		} else {
			x.Common.Algorithms = strings.Split(x.Algorithms, ",")
		}

		// 登录方式
		if x.LoginType == "refresh_token" {
			// 通过RefreshToken登录
			token, err := x.XunLeiCommon.RefreshToken(x.ExpertAddition.RefreshToken)
			if err != nil {
				return err
			}
			x.SetTokenResp(token)

			// 刷新token方法
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})
		} else {
			// 通过用户密码登录
			token, err := x.Login(x.Username, x.Password)
			if err != nil {
				return err
			}
			// 清空 信任密钥
			x.ExpertAddition.CreditKey = ""
			x.SetTokenResp(token)
			x.SetRefreshTokenFunc(func() error {
				token, err := x.XunLeiCommon.RefreshToken(x.TokenResp.RefreshToken)
				if err != nil {
					token, err = x.Login(x.Username, x.Password)
					if err != nil {
						x.GetStorage().SetStatus(fmt.Sprintf("%+v", err.Error()))
					}
					// 清空 信任密钥
					x.ExpertAddition.CreditKey = ""
				}
				x.SetTokenResp(token)
				op.MustSaveDriverStorage(x)
				return err
			})
		}
	} else {
		// 仅修改验证码token
		if x.CaptchaToken != "" {
			x.SetCaptchaToken(x.CaptchaToken)
		}
		x.XunLeiCommon.UserAgent = x.UserAgent
		x.XunLeiCommon.DownloadUserAgent = x.DownloadUserAgent
		x.XunLeiCommon.UseVideoUrl = x.UseVideoUrl
	}
	return nil
}

func (x *ThunderExpert) Drop(ctx context.Context) error {
	return nil
}

func (x *ThunderExpert) SetTokenResp(token *TokenResp) {
	x.XunLeiCommon.SetTokenResp(token)
	if token != nil {
		x.ExpertAddition.RefreshToken = token.RefreshToken
	}
}

type XunLeiCommon struct {
	*Common
	*TokenResp     // 登录信息
	*CoreLoginResp // core登录信息

	refreshTokenFunc func() error
}

func (xc *XunLeiCommon) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	return xc.getFiles(ctx, dir.GetID())
}

func (xc *XunLeiCommon) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	var lFile Files
	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodGet, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", file.GetID())
		//r.SetQueryParam("space", "")
	}, &lFile)
	if err != nil {
		return nil, err
	}
	link := &model.Link{
		URL: lFile.WebContentLink,
		Header: http.Header{
			"User-Agent": {xc.DownloadUserAgent},
		},
	}

	if xc.UseVideoUrl {
		for _, media := range lFile.Medias {
			if media.Link.URL != "" {
				link.URL = media.Link.URL
				break
			}
		}
	}

	/*
		strs := regexp.MustCompile(`e=([0-9]*)`).FindStringSubmatch(lFile.WebContentLink)
		if len(strs) == 2 {
			timestamp, err := strconv.ParseInt(strs[1], 10, 64)
			if err == nil {
				expired := time.Duration(timestamp-time.Now().Unix()) * time.Second
				link.Expiration = &expired
			}
		}
	*/
	return link, nil
}

func (xc *XunLeiCommon) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"kind":      FOLDER,
			"name":      dirName,
			"parent_id": parentDir.GetID(),
		})
	}, nil)
	return err
}

func (xc *XunLeiCommon) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := xc.Request(FILE_API_URL+":batchMove", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"to":  base.Json{"parent_id": dstDir.GetID()},
			"ids": []string{srcObj.GetID()},
		})
	}, nil)
	return err
}

func (xc *XunLeiCommon) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	_, err := xc.Request(FILE_API_URL+"/{fileID}", http.MethodPatch, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", srcObj.GetID())
		r.SetBody(&base.Json{"name": newName})
	}, nil)
	return err
}

func (xc *XunLeiCommon) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	_, err := xc.Request(FILE_API_URL+":batchCopy", http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"to":  base.Json{"parent_id": dstDir.GetID()},
			"ids": []string{srcObj.GetID()},
		})
	}, nil)
	return err
}

func (xc *XunLeiCommon) Remove(ctx context.Context, obj model.Obj) error {
	_, err := xc.Request(FILE_API_URL+"/{fileID}/trash", http.MethodPatch, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetPathParam("fileID", obj.GetID())
		r.SetBody("{}")
	}, nil)
	return err
}

func (xc *XunLeiCommon) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	gcid := file.GetHash().GetHash(hash_extend.GCID)
	var err error
	if len(gcid) < hash_extend.GCID.Width {
		_, gcid, err = stream.CacheFullInTempFileAndHash(file, hash_extend.GCID, file.GetSize())
		if err != nil {
			return err
		}
	}

	var resp UploadTaskResponse
	_, err = xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"kind":        FILE,
			"parent_id":   dstDir.GetID(),
			"name":        file.GetName(),
			"size":        file.GetSize(),
			"hash":        gcid,
			"upload_type": UPLOAD_TYPE_RESUMABLE,
		})
	}, &resp)
	if err != nil {
		return err
	}

	param := resp.Resumable.Params
	if resp.UploadType == UPLOAD_TYPE_RESUMABLE {
		param.Endpoint = strings.TrimLeft(param.Endpoint, param.Bucket+".")
		s, err := session.NewSession(&aws.Config{
			Credentials: credentials.NewStaticCredentials(param.AccessKeyID, param.AccessKeySecret, param.SecurityToken),
			Region:      aws.String("xunlei"),
			Endpoint:    aws.String(param.Endpoint),
		})
		if err != nil {
			return err
		}
		uploader := s3manager.NewUploader(s)
		if file.GetSize() > s3manager.MaxUploadParts*s3manager.DefaultUploadPartSize {
			uploader.PartSize = file.GetSize() / (s3manager.MaxUploadParts - 1)
		}
		_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket:  aws.String(param.Bucket),
			Key:     aws.String(param.Key),
			Expires: aws.Time(param.Expiration),
			Body: driver.NewLimitedUploadStream(ctx, &driver.ReaderUpdatingProgress{
				Reader:         file,
				UpdateProgress: up,
			}),
		})
		return err
	}
	return nil
}

func (xc *XunLeiCommon) getFiles(ctx context.Context, folderId string) ([]model.Obj, error) {
	files := make([]model.Obj, 0)
	var pageToken string
	for {
		var fileList FileList
		_, err := xc.Request(FILE_API_URL, http.MethodGet, func(r *resty.Request) {
			r.SetContext(ctx)
			r.SetQueryParams(map[string]string{
				"space":      "",
				"__type":     "drive",
				"refresh":    "true",
				"__sync":     "true",
				"parent_id":  folderId,
				"page_token": pageToken,
				"with_audit": "true",
				"limit":      "100",
				"filters":    `{"phase":{"eq":"PHASE_TYPE_COMPLETE"},"trashed":{"eq":false}}`,
			})
		}, &fileList)
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(fileList.Files); i++ {
			files = append(files, &fileList.Files[i])
		}

		if fileList.NextPageToken == "" {
			break
		}
		pageToken = fileList.NextPageToken
	}
	return files, nil
}

// 设置刷新Token的方法
func (xc *XunLeiCommon) SetRefreshTokenFunc(fn func() error) {
	xc.refreshTokenFunc = fn
}

// 设置Token
func (xc *XunLeiCommon) SetTokenResp(tr *TokenResp) {
	xc.TokenResp = tr
}

func (xc *XunLeiCommon) SetCoreTokenResp(tr *CoreLoginResp) {
	xc.CoreLoginResp = tr
}

// 携带Authorization和CaptchaToken的请求
func (xc *XunLeiCommon) Request(url string, method string, callback base.ReqCallback, resp interface{}) ([]byte, error) {
	data, err := xc.Common.Request(url, method, func(req *resty.Request) {
		req.SetHeaders(map[string]string{
			"Authorization":   xc.Token(),
			"X-Captcha-Token": xc.GetCaptchaToken(),
		})
		if callback != nil {
			callback(req)
		}
	}, resp)

	errResp, ok := err.(*ErrResp)
	if !ok {
		return nil, err
	}

	switch errResp.ErrorCode {
	case 0:
		return data, nil
	case 4122, 4121, 10, 16:
		if xc.refreshTokenFunc != nil {
			if err = xc.refreshTokenFunc(); err == nil {
				break
			}
		}
		return nil, err
	case 9: // 验证码token过期
		if err = xc.RefreshCaptchaTokenAtLogin(GetAction(method, url), xc.TokenResp.UserID); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return xc.Request(url, method, callback, resp)
}

// 刷新Token
func (xc *XunLeiCommon) RefreshToken(refreshToken string) (*TokenResp, error) {
	var resp TokenResp
	_, err := xc.Common.Request(XLUSER_API_URL+"/auth/token", http.MethodPost, func(req *resty.Request) {
		req.SetBody(&base.Json{
			"grant_type":    "refresh_token",
			"refresh_token": refreshToken,
			"client_id":     xc.ClientID,
			"client_secret": xc.ClientSecret,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}

	if resp.RefreshToken == "" {
		return nil, errs.EmptyToken
	}
	return &resp, nil
}

// 登录
func (xc *XunLeiCommon) Login(username, password string) (*TokenResp, error) {
	//v3 login拿到 sessionID
	sessionID, err := xc.CoreLogin(username, password)
	if err != nil {
		return nil, err
	}
	//v1 login拿到令牌
	url := XLUSER_API_URL + "/auth/signin/token"
	if err = xc.RefreshCaptchaTokenInLogin(GetAction(http.MethodPost, url), username); err != nil {
		return nil, err
	}

	var resp TokenResp
	_, err = xc.Common.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetPathParam("client_id", xc.ClientID)
		req.SetBody(&SignInRequest{
			ClientID:     xc.ClientID,
			ClientSecret: xc.ClientSecret,
			Provider:     SignProvider,
			SigninToken:  sessionID,
		})
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (xc *XunLeiCommon) IsLogin() bool {
	if xc.TokenResp == nil {
		return false
	}
	_, err := xc.Request(XLUSER_API_URL+"/user/me", http.MethodGet, nil, nil)
	return err == nil
}

// 离线下载文件
func (xc *XunLeiCommon) OfflineDownload(ctx context.Context, fileUrl string, parentDir model.Obj, fileName string) (*OfflineTask, error) {
	var resp OfflineDownloadResp
	_, err := xc.Request(FILE_API_URL, http.MethodPost, func(r *resty.Request) {
		r.SetContext(ctx)
		r.SetBody(&base.Json{
			"kind":        FILE,
			"name":        fileName,
			"parent_id":   parentDir.GetID(),
			"upload_type": UPLOAD_TYPE_URL,
			"url": base.Json{
				"url": fileUrl,
			},
		})
	}, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.Task, err
}

/*
获取离线下载任务列表
*/
func (xc *XunLeiCommon) OfflineList(ctx context.Context, nextPageToken string) ([]OfflineTask, error) {
	res := make([]OfflineTask, 0)

	var resp OfflineListResp
	_, err := xc.Request(TASK_API_URL, http.MethodGet, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(map[string]string{
				"type":       "offline",
				"limit":      "10000",
				"page_token": nextPageToken,
			})
	}, &resp)

	if err != nil {
		return nil, fmt.Errorf("failed to get offline list: %w", err)
	}
	res = append(res, resp.Tasks...)
	return res, nil
}

func (xc *XunLeiCommon) DeleteOfflineTasks(ctx context.Context, taskIDs []string, deleteFiles bool) error {
	_, err := xc.Request(TASK_API_URL, http.MethodDelete, func(req *resty.Request) {
		req.SetContext(ctx).
			SetQueryParams(map[string]string{
				"task_ids":     strings.Join(taskIDs, ","),
				"delete_files": strconv.FormatBool(deleteFiles),
			})
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to delete tasks %v: %w", taskIDs, err)
	}
	return nil
}

func (xc *XunLeiCommon) CoreLogin(username string, password string) (sessionID string, err error) {
	url := XLUSER_API_BASE_URL + "/xluser.core.login/v3/login"
	var resp CoreLoginResp
	res, err := xc.Common.Request(url, http.MethodPost, func(req *resty.Request) {
		req.SetHeader("User-Agent", "android-ok-http-client/xl-acc-sdk/version-5.0.12.512000")
		req.SetBody(&CoreLoginRequest{
			ProtocolVersion: "301",
			SequenceNo:      "1000012",
			PlatformVersion: "10",
			IsCompressed:    "0",
			Appid:           APPID,
			ClientVersion:   "8.31.0.9726",
			PeerID:          "00000000000000000000000000000000",
			AppName:         "ANDROID-com.xunlei.downloadprovider",
			SdkVersion:      "512000",
			Devicesign:      generateDeviceSign(xc.DeviceID, xc.PackageName),
			NetWorkType:     "WIFI",
			ProviderName:    "NONE",
			DeviceModel:     "M2004J7AC",
			DeviceName:      "Xiaomi_M2004j7ac",
			OSVersion:       "12",
			Creditkey:       xc.GetCreditKey(),
			Hl:              "zh-CN",
			UserName:        username,
			PassWord:        password,
			VerifyKey:       "",
			VerifyCode:      "",
			IsMd5Pwd:        "0",
		})
	}, nil)
	if err != nil {
		return "", err
	}

	if err = utils.Json.Unmarshal(res, &resp); err != nil {
		return "", err
	}

	xc.SetCoreTokenResp(&resp)

	sessionID = resp.SessionID

	return sessionID, nil
}
