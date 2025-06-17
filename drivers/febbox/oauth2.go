package febbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type customTokenSource struct {
	config       *clientcredentials.Config
	ctx          context.Context
	refreshToken string
}

func (c *customTokenSource) Token() (*oauth2.Token, error) {
	v := url.Values{}
	if c.refreshToken != "" {
		v.Set("grant_type", "refresh_token")
		v.Set("refresh_token", c.refreshToken)
	} else {
		v.Set("grant_type", "client_credentials")
	}

	v.Set("client_id", c.config.ClientID)
	v.Set("client_secret", c.config.ClientSecret)

	req, err := http.NewRequest("POST", c.config.TokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req.WithContext(c.ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("oauth2: cannot fetch token")
	}

	var tokenResp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			AccessToken  string `json:"access_token"`
			ExpiresIn    int64  `json:"expires_in"`
			TokenType    string `json:"token_type"`
			Scope        string `json:"scope"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.Code != 1 {
		return nil, errors.New("oauth2: server response error")
	}

	c.refreshToken = tokenResp.Data.RefreshToken

	token := &oauth2.Token{
		AccessToken:  tokenResp.Data.AccessToken,
		TokenType:    tokenResp.Data.TokenType,
		RefreshToken: tokenResp.Data.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(tokenResp.Data.ExpiresIn) * time.Second),
	}

	return token, nil
}

func (d *FebBox) initializeOAuth2Token(ctx context.Context, oauth2Config *clientcredentials.Config, refreshToken string) {
	d.oauth2Token = oauth2.ReuseTokenSource(nil, &customTokenSource{
		config:       oauth2Config,
		ctx:          ctx,
		refreshToken: refreshToken,
	})
}
