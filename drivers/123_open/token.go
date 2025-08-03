package _123Open

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const tokenURL = ApiBaseURL + ApiToken

type tokenManager struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	accessToken string
	expireTime  time.Time
}

func newTokenManager(clientID, clientSecret string) *tokenManager {
	return &tokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (tm *tokenManager) getToken() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.accessToken != "" && time.Now().Before(tm.expireTime.Add(-5*time.Minute)) {
		return tm.accessToken, nil
	}

	reqBody := map[string]string{
		"clientID":     tm.clientID,
		"clientSecret": tm.clientSecret,
	}
	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Platform", "open_platform")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result TokenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("get token failed: %s", result.Message)
	}

	tm.accessToken = result.Data.AccessToken
	expireAt, err := time.Parse(time.RFC3339, result.Data.ExpiredAt)
	if err != nil {
		return "", fmt.Errorf("parse expire time failed: %w", err)
	}
	tm.expireTime = expireAt

	return tm.accessToken, nil
}

func (tm *tokenManager) buildHeaders() (http.Header, error) {
	token, err := tm.getToken()
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("Platform", "open_platform")
	header.Set("Content-Type", "application/json")
	return header, nil
}
