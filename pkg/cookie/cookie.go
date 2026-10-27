package cookie

import (
	"net/http"
	"strings"
)

func Parse(str string) []*http.Cookie {
	header := http.Header{}
	header.Add("Cookie", str)
	request := http.Request{Header: header}
	return request.Cookies()
}

func ToString(cookies []*http.Cookie) string {
	if cookies == nil {
		return ""
	}
	cookieStrings := make([]string, len(cookies))
	for i, cookie := range cookies {
		cookieStrings[i] = cookie.String()
	}
	return strings.Join(cookieStrings, ";")
}

func SetCookie(cookies []*http.Cookie, name, value string) []*http.Cookie {
	for i, cookie := range cookies {
		if cookie.Name == name {
			cookies[i].Value = value
			return cookies
		}
	}
	cookies = append(cookies, &http.Cookie{Name: name, Value: value})
	return cookies
}

func GetCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func SetStr(cookiesStr, name, value string) string {
	cookies := Parse(cookiesStr)
	cookies = SetCookie(cookies, name, value)
	return ToString(cookies)
}

func GetStr(cookiesStr, name string) string {
	cookies := Parse(cookiesStr)
	cookie := GetCookie(cookies, name)
	if cookie == nil {
		return ""
	}
	return cookie.Value
}
