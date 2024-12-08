package handles

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Xhofe/go-cache"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/coreos/go-oidc"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const stateLength = 16
const stateExpire = time.Minute * 5

var stateCache = cache.NewMemCache[string](cache.WithShards[string](stateLength))

func _keyState(clientID, state string) string {
	return fmt.Sprintf("%s_%s", clientID, state)
}

func generateState(clientID, ip string) string {
	state := random.String(stateLength)
	stateCache.Set(_keyState(clientID, state), ip, cache.WithEx[string](stateExpire))
	return state
}

func verifyState(clientID, ip, state string) bool {
	value, ok := stateCache.Get(_keyState(clientID, state))
	return ok && value == ip
}

func ssoRedirectUri(c *gin.Context, useCompatibility bool, method string) string {
	if useCompatibility {
		return common.GetApiUrl(c.Request) + "/api/auth/" + method
	} else {
		return common.GetApiUrl(c.Request) + "/api/auth/sso_callback" + "?method=" + method
	}
}

func SSOLoginRedirect(c *gin.Context) {
	method := c.Query("method")
	useCompatibility := setting.GetBool(conf.SSOCompatibilityMode)
	enabled := setting.GetBool(conf.SSOLoginEnabled)
	clientId := setting.GetStr(conf.SSOClientId)
	platform := setting.GetStr(conf.SSOLoginPlatform)
	var rUrl string
	if !enabled {
		common.ErrorStrResp(c, "Single sign-on is not enabled", 403)
		return
	}
	urlValues := url.Values{}
	if method == "" {
		common.ErrorStrResp(c, "no method provided", 400)
		return
	}
	redirectUri := ssoRedirectUri(c, useCompatibility, method)
	urlValues.Add("response_type", "code")
	urlValues.Add("redirect_uri", redirectUri)
	urlValues.Add("client_id", clientId)
	switch platform {
	case "Github":
		rUrl = "https://github.com/login/oauth/authorize?"
		urlValues.Add("scope", "read:user")
	case "Microsoft":
		rUrl = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?"
		urlValues.Add("scope", "user.read")
		urlValues.Add("response_mode", "query")
	case "Google":
		rUrl = "https://accounts.google.com/o/oauth2/v2/auth?"
		urlValues.Add("scope", "https://www.googleapis.com/auth/userinfo.profile")
	case "Dingtalk":
		rUrl = "https://login.dingtalk.com/oauth2/auth?"
		urlValues.Add("scope", "openid")
		urlValues.Add("prompt", "consent")
		urlValues.Add("response_type", "code")
	case "Casdoor":
		endpoint := strings.TrimSuffix(setting.GetStr(conf.SSOEndpointName), "/")
		rUrl = endpoint + "/login/oauth/authorize?"
		urlValues.Add("scope", "profile")
		urlValues.Add("state", endpoint)
	case "OIDC":
		oauth2Config, err := GetOIDCClient(c, useCompatibility, redirectUri, method)
		if err != nil {
			common.ErrorStrResp(c, err.Error(), 400)
			return
		}
		state := generateState(clientId, c.ClientIP())
		c.Redirect(http.StatusFound, oauth2Config.AuthCodeURL(state))
		return
	default:
		common.ErrorStrResp(c, "invalid platform", 400)
		return
	}
	c.Redirect(302, rUrl+urlValues.Encode())
}

var ssoClient = resty.New().SetRetryCount(3)

func GetOIDCClient(c *gin.Context, useCompatibility bool, redirectUri, method string) (*oauth2.Config, error) {
	if redirectUri == "" {
		redirectUri = ssoRedirectUri(c, useCompatibility, method)
	}
	endpoint := setting.GetStr(conf.SSOEndpointName)
	provider, err := oidc.NewProvider(c, endpoint)
	if err != nil {
		return nil, err
	}
	clientId := setting.GetStr(conf.SSOClientId)
	clientSecret := setting.GetStr(conf.SSOClientSecret)
	extraScopes := []string{}
	if setting.GetStr(conf.SSOExtraScopes) != "" {
		extraScopes = strings.Split(setting.GetStr(conf.SSOExtraScopes), " ")
	}
	return &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		RedirectURL:  redirectUri,

		// Discovery returns the OAuth2 endpoints.
		Endpoint: provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		Scopes: append([]string{oidc.ScopeOpenID, "profile"}, extraScopes...),
	}, nil
}

func autoRegister(username, userID string, err error) (*model.User, error) {
	if !errors.Is(err, gorm.ErrRecordNotFound) || !setting.GetBool(conf.SSOAutoRegister) {
		return nil, err
	}
	if username == "" {
		return nil, errors.New("cannot get username from SSO provider")
	}
	user := &model.User{
		ID:         0,
		Username:   username,
		Password:   random.String(16),
		Permission: int32(setting.GetInt(conf.SSODefaultPermission, 0)),
		BasePath:   setting.GetStr(conf.SSODefaultDir),
		Role:       0,
		Disabled:   false,
		SsoID:      userID,
	}
	if err = db.CreateUser(user); err != nil {
		if strings.HasPrefix(err.Error(), "UNIQUE constraint failed") && strings.HasSuffix(err.Error(), "username") {
			user.Username = user.Username + "_" + userID
			if err = db.CreateUser(user); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return user, nil
}

func parseJWT(p string) ([]byte, error) {
	parts := strings.Split(p, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("oidc: malformed jwt, expected 3 parts got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("oidc: malformed jwt payload: %v", err)
	}
	return payload, nil
}

func OIDCLoginCallback(c *gin.Context) {
	useCompatibility := setting.GetBool(conf.SSOCompatibilityMode)
	method := c.Query("method")
	if useCompatibility {
		method = path.Base(c.Request.URL.Path)
	}
	clientId := setting.GetStr(conf.SSOClientId)
	endpoint := setting.GetStr(conf.SSOEndpointName)
	provider, err := oidc.NewProvider(c, endpoint)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	oauth2Config, err := GetOIDCClient(c, useCompatibility, "", method)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if !verifyState(clientId, c.ClientIP(), c.Query("state")) {
		common.ErrorStrResp(c, "incorrect or expired state parameter", 400)
		return
	}

	oauth2Token, err := oauth2Config.Exchange(c, c.Query("code"))
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		common.ErrorStrResp(c, "no id_token found in oauth2 token", 400)
		return
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID: clientId,
	})
	_, err = verifier.Verify(c, rawIDToken)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	payload, err := parseJWT(rawIDToken)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	userID := utils.Json.Get(payload, setting.GetStr(conf.SSOOIDCUsernameKey, "name")).ToString()
	if userID == "" {
		common.ErrorStrResp(c, "cannot get username from OIDC provider", 400)
		return
	}
	if method == "get_sso_id" {
		if useCompatibility {
			c.Redirect(302, common.GetApiUrl(c.Request)+"/@manage?sso_id="+userID)
			return
		}
		html := fmt.Sprintf(`<!DOCTYPE html>
				<head></head>
				<body>
				<script>
				window.opener.postMessage({"sso_id": "%s"}, "*")
				window.close()
				</script>
				</body>`, userID)
		c.Data(200, "text/html; charset=utf-8", []byte(html))
		return
	}
	if method == "sso_get_token" {
		user, err := db.GetUserBySSOID(userID)
		if err != nil {
			user, err = autoRegister(userID, userID, err)
			if err != nil {
				common.ErrorResp(c, err, 400)
			}
		}
		token, err := common.GenerateToken(user)
		if err != nil {
			common.ErrorResp(c, err, 400)
		}
		if useCompatibility {
			c.Redirect(302, common.GetApiUrl(c.Request)+"/@login?token="+token)
			return
		}
		html := fmt.Sprintf(`<!DOCTYPE html>
				<head></head>
				<body>
				<script>
				window.opener.postMessage({"token":"%s"}, "*")
				window.close()
				</script>
				</body>`, token)
		c.Data(200, "text/html; charset=utf-8", []byte(html))
		return
	}
}

func SSOLoginCallback(c *gin.Context) {
	enabled := setting.GetBool(conf.SSOLoginEnabled)
	usecompatibility := setting.GetBool(conf.SSOCompatibilityMode)
	if !enabled {
		common.ErrorResp(c, errors.New("sso login is disabled"), 500)
		return
	}
	argument := c.Query("method")
	if usecompatibility {
		argument = path.Base(c.Request.URL.Path)
	}
	if !utils.SliceContains([]string{"get_sso_id", "sso_get_token"}, argument) {
		common.ErrorResp(c, errors.New("invalid request"), 500)
		return
	}
	clientId := setting.GetStr(conf.SSOClientId)
	platform := setting.GetStr(conf.SSOLoginPlatform)
	clientSecret := setting.GetStr(conf.SSOClientSecret)
	var tokenUrl, userUrl, scope, authField, idField, usernameField string
	additionalForm := make(map[string]string)
	switch platform {
	case "Github":
		tokenUrl = "https://github.com/login/oauth/access_token"
		userUrl = "https://api.github.com/user"
		authField = "code"
		scope = "read:user"
		idField = "id"
		usernameField = "login"
	case "Microsoft":
		tokenUrl = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
		userUrl = "https://graph.microsoft.com/v1.0/me"
		additionalForm["grant_type"] = "authorization_code"
		scope = "user.read"
		authField = "code"
		idField = "id"
		usernameField = "displayName"
	case "Google":
		tokenUrl = "https://oauth2.googleapis.com/token"
		userUrl = "https://www.googleapis.com/oauth2/v1/userinfo"
		additionalForm["grant_type"] = "authorization_code"
		scope = "https://www.googleapis.com/auth/userinfo.profile"
		authField = "code"
		idField = "id"
		usernameField = "name"
	case "Dingtalk":
		tokenUrl = "https://api.dingtalk.com/v1.0/oauth2/userAccessToken"
		userUrl = "https://api.dingtalk.com/v1.0/contact/users/me"
		authField = "authCode"
		idField = "unionId"
		usernameField = "nick"
	case "Casdoor":
		endpoint := strings.TrimSuffix(setting.GetStr(conf.SSOEndpointName), "/")
		tokenUrl = endpoint + "/api/login/oauth/access_token"
		userUrl = endpoint + "/api/userinfo"
		additionalForm["grant_type"] = "authorization_code"
		scope = "profile"
		authField = "code"
		idField = "sub"
		usernameField = "preferred_username"
	case "OIDC":
		OIDCLoginCallback(c)
		return
	default:
		common.ErrorStrResp(c, "invalid platform", 400)
		return
	}
	callbackCode := c.Query(authField)
	if callbackCode == "" {
		common.ErrorStrResp(c, "No code provided", 400)
		return
	}
	var resp *resty.Response
	var err error
	if platform == "Dingtalk" {
		resp, err = ssoClient.R().SetHeader("content-type", "application/json").SetHeader("Accept", "application/json").
			SetBody(map[string]string{
				"clientId":     clientId,
				"clientSecret": clientSecret,
				"code":         callbackCode,
				"grantType":    "authorization_code",
			}).
			Post(tokenUrl)
	} else {
		var redirect_uri string
		if usecompatibility {
			redirect_uri = common.GetApiUrl(c.Request) + "/api/auth/" + argument
		} else {
			redirect_uri = common.GetApiUrl(c.Request) + "/api/auth/sso_callback" + "?method=" + argument
		}
		resp, err = ssoClient.R().SetHeader("Accept", "application/json").
			SetFormData(map[string]string{
				"client_id":     clientId,
				"client_secret": clientSecret,
				"code":          callbackCode,
				"redirect_uri":  redirect_uri,
				"scope":         scope,
			}).SetFormData(additionalForm).Post(tokenUrl)
	}
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if platform == "Dingtalk" {
		accessToken := utils.Json.Get(resp.Body(), "accessToken").ToString()
		resp, err = ssoClient.R().SetHeader("x-acs-dingtalk-access-token", accessToken).
			Get(userUrl)
	} else {
		accessToken := utils.Json.Get(resp.Body(), "access_token").ToString()
		resp, err = ssoClient.R().SetHeader("Authorization", "Bearer "+accessToken).
			Get(userUrl)
	}
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	userID := utils.Json.Get(resp.Body(), idField).ToString()
	if utils.SliceContains([]string{"", "0"}, userID) {
		common.ErrorResp(c, errors.New("error occurred"), 400)
		return
	}
	if argument == "get_sso_id" {
		if usecompatibility {
			c.Redirect(302, common.GetApiUrl(c.Request)+"/@manage?sso_id="+userID)
			return
		}
		html := fmt.Sprintf(`<!DOCTYPE html>
				<head></head>
				<body>
				<script>
				window.opener.postMessage({"sso_id": "%s"}, "*")
				window.close()
				</script>
				</body>`, userID)
		c.Data(200, "text/html; charset=utf-8", []byte(html))
		return
	}
	username := utils.Json.Get(resp.Body(), usernameField).ToString()
	user, err := db.GetUserBySSOID(userID)
	if err != nil {
		user, err = autoRegister(username, userID, err)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
	}
	token, err := common.GenerateToken(user)
	if err != nil {
		common.ErrorResp(c, err, 400)
	}
	if usecompatibility {
		c.Redirect(302, common.GetApiUrl(c.Request)+"/@login?token="+token)
		return
	}
	html := fmt.Sprintf(`<!DOCTYPE html>
							<head></head>
							<body>
							<script>
							window.opener.postMessage({"token":"%s"}, "*")
							window.close()
							</script>
							</body>`, token)
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}
