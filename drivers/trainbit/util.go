package trainbit

import (
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/alist-org/alist/v3/internal/model"
)

func get(url string, apiKey string, AUSHELLPORTAL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{
		Name:   ".AUSHELLPORTAL",
		Value:  AUSHELLPORTAL,
		MaxAge: 2 * 60,
	})
	req.AddCookie(&http.Cookie{
		Name:   "retkeyapi",
		Value:  apiKey,
		MaxAge: 2 * 60,
	})
	res, err := base.HttpClient.Do(req)
	return res, err
}

func postForm(endpoint string, data url.Values, apiExpiredate string, apiKey string, AUSHELLPORTAL string) (*http.Response, error) {
	extData := make(url.Values)
	for key, value := range data {
		extData[key] = make([]string, len(value))
		copy(extData[key], value)
	}
	extData.Set("apikey", apiKey)
	extData.Set("expiredate", apiExpiredate)
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(extData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:   ".AUSHELLPORTAL",
		Value:  AUSHELLPORTAL,
		MaxAge: 2 * 60,
	})
	req.AddCookie(&http.Cookie{
		Name:   "retkeyapi",
		Value:  apiKey,
		MaxAge: 2 * 60,
	})
	res, err := base.HttpClient.Do(req)
	return res, err
}

func getToken(apiKey string, AUSHELLPORTAL string) (string, string, error) {
	res, err := get("https://trainbit.com/files/", apiKey, AUSHELLPORTAL)
	if err != nil {
		return "", "", err
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", err
	}
	text := string(data)
	apiExpiredateReg := regexp.MustCompile(`core.api.expiredate = '([^']*)';`)
	result := apiExpiredateReg.FindAllStringSubmatch(text, -1)
	apiExpiredate := result[0][1]
	guidReg := regexp.MustCompile(`app.vars.upload.guid = '([^']*)';`)
	result = guidReg.FindAllStringSubmatch(text, -1)
	guid := result[0][1]
	return apiExpiredate, guid, nil
}

func local2provider(filename string, isFolder bool) string {
	if isFolder {
		return filename
	}
	return filename + ".delete_suffix"
}

func provider2local(filename string) string {
	filename = html.UnescapeString(filename)
	index := strings.LastIndex(filename, ".delete_suffix")
	if index != -1 {
		filename = filename[:index]
	}
	return filename
}

func parseRawFileObject(rawObject []any) ([]model.Obj, error) {
	objectList := make([]model.Obj, 0)
	for _, each := range rawObject {
		object := each.(map[string]any)
		if object["id"].(string) == "0" {
			continue
		}
		isFolder := int64(object["ty"].(float64)) == 1
		var name string
		if object["ext"].(string) != "" {
			name = strings.Join([]string{object["name"].(string), object["ext"].(string)}, ".")
		} else {
			name = object["name"].(string)
		}
		modified, err := time.Parse("2006/01/02 15:04:05", object["modified"].(string))
		if err != nil {
			return nil, err
		}
		objectList = append(objectList, model.Obj(&model.Object{
			ID:       strings.Join([]string{object["id"].(string), strings.Split(object["uploadurl"].(string), "=")[1]}, "_"),
			Name:     provider2local(name),
			Size:     int64(object["byte"].(float64)),
			Modified: modified.Add(-210 * time.Minute),
			IsFolder: isFolder,
		}))
	}
	return objectList, nil
}
