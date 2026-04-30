package guangyapan

import (
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/op"
)

type Addition struct {
	RootPath       string `json:"root_path" help:"光鸭云盘中的完整路径"`
	PhoneNumber    string `json:"phone_number" type:"text" help:"Phone number for SMS login, e.g. +86 13800000000"`
	CaptchaToken   string `json:"captcha_token" type:"text" help:"Captcha token required by /v1/auth/verification"`
	SendCode       bool   `json:"send_code" type:"bool" help:"Set true and save to send SMS code, it auto-resets to false after sending"`
	VerifyCode     string `json:"verify_code" type:"text" help:"SMS verification code used with phone_number; fill then save to finish login"`
	VerificationID string `json:"verification_id" type:"text" help:"Auto-generated after sending SMS code; do not edit manually"`
	AccessToken    string `json:"access_token" type:"text" help:"Bearer access token (optional if refresh_token is provided)"`
	RefreshToken   string `json:"refresh_token" type:"text" help:"Refresh token for auto-login/auto-refresh"`
	ClientID       string `json:"client_id" default:"aMe-8VSlkrbQXpUR"`
	DeviceID       string `json:"device_id" help:"Optional custom device id (32 hex chars), auto-generated when empty"`
	PageSize       int    `json:"page_size" type:"number" default:"100"`
	OrderBy        int    `json:"order_by" type:"number" default:"3" help:"0:name,1:size,2:create_time,3:update_time"`
	SortType       int    `json:"sort_type" type:"number" default:"1" help:"0:asc,1:desc"`
}

var config = driver.Config{
	Name:              "GuangYaPan",
	LocalSort:         false,
	OnlyLocal:         false,
	OnlyProxy:         false,
	NoCache:           false,
	NoUpload:          false,
	NeedMs:            false,
	DefaultRoot:       "",
	CheckStatus:       true,
	Alert:             "info|Two-stage SMS login: (1) fill phone_number (+ captcha_token if needed), set send_code=true and save; (2) fill verify_code and save to finish login and auto-save access_token/refresh_token.",
	NoOverwriteUpload: true,
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &GuangYaPan{}
	})
}