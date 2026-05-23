package google_photo

import (
	"fmt"
	"net/http"

	"github.com/alist-org/alist/v3/drivers/base"
	"github.com/go-resty/resty/v2"
)

// do others that not defined in Driver interface

const (
	FETCH_ALL = "all"
	FETCH_ALBUMS = "albums"
	FETCH_ROOT = "root"
	FETCH_SHARE_ALBUMS = "share_albums"
)

func (d *GooglePhoto) refreshToken() error {
	url := "https://www.googleapis.com/oauth2/v4/token"
	var resp base.TokenResp
	var e TokenError
	_, err := base.RestyClient.R().SetResult(&resp).SetError(&e).
		SetFormData(map[string]string{
			"client_id":     d.ClientID,
			"client_secret": d.ClientSecret,
			"refresh_token": d.RefreshToken,
			"grant_type":    "refresh_token",
		}).Post(url)
	if err != nil {
		return err
	}
	if e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	d.AccessToken = resp.AccessToken
	return nil
}

func (d *GooglePhoto) request(url string, method string, callback base.ReqCallback, resp interface{}, headers map[string]string) ([]byte, error) {
	req := base.RestyClient.R()
	req.SetHeader("Authorization", "Bearer "+d.AccessToken)
	req.SetHeader("Accept-Encoding", "gzip")
	if headers != nil {
		req.SetHeaders(headers)
	}

	if callback != nil {
		callback(req)
	}
	if resp != nil {
		req.SetResult(resp)
	}
	var e Error
	req.SetError(&e)
	res, err := req.Execute(method, url)
	if err != nil {
		return nil, err
	}
	if e.Error.Code != 0 {
		if e.Error.Code == 401 {
			err = d.refreshToken()
			if err != nil {
				return nil, err
			}
			return d.request(url, method, callback, resp, headers)
		}
		return nil, fmt.Errorf("%s: %v", e.Error.Message, e.Error.Errors)
	}
	return res.Body(), nil
}

func (d *GooglePhoto) getFiles(id string) ([]MediaItem, error) {
	switch id {
	case FETCH_ALL:
		return d.getAllMedias()
	case FETCH_ALBUMS:
		return d.getAlbums()
	case FETCH_SHARE_ALBUMS:
		return d.getShareAlbums()
	case FETCH_ROOT:
		return d.getFakeRoot()
	default:
		return d.getMedias(id)
	}
}

func (d *GooglePhoto) getFakeRoot() ([]MediaItem, error) {
	return []MediaItem{
		{
			Id: FETCH_ALL,
			Title: "全部媒体",
		},
		{
			Id: FETCH_ALBUMS,
			Title: "全部影集",
		},
		{
			Id: FETCH_SHARE_ALBUMS,
			Title: "共享影集",
		},
	}, nil
}

func (d *GooglePhoto) getAlbums() ([]MediaItem, error) {
	return d.fetchItems(
		"https://photoslibrary.googleapis.com/v1/albums",
		map[string]string{
			"fields":    "albums(id,title,coverPhotoBaseUrl),nextPageToken",
			"pageSize":  "50",
			"pageToken": "first",
		},
		http.MethodGet)
}

func (d *GooglePhoto) getShareAlbums() ([]MediaItem, error) {
	return d.fetchItems(
		"https://photoslibrary.googleapis.com/v1/sharedAlbums",
		map[string]string{
			"fields":    "sharedAlbums(id,title,coverPhotoBaseUrl),nextPageToken",
			"pageSize":  "50",
			"pageToken": "first",
		},
		http.MethodGet)
}

func (d *GooglePhoto) getMedias(albumId string) ([]MediaItem, error) {
	return d.fetchItems(
		"https://photoslibrary.googleapis.com/v1/mediaItems:search",
		map[string]string{
			"fields":    "mediaItems(id,baseUrl,mimeType,mediaMetadata,filename),nextPageToken",
			"pageSize":  "100",
			"albumId": albumId,
			"pageToken": "first",
		}, http.MethodPost)
}

func (d *GooglePhoto) getAllMedias() ([]MediaItem, error) {
	return d.fetchItems(
		"https://photoslibrary.googleapis.com/v1/mediaItems",
		map[string]string{
			"fields":    "mediaItems(id,baseUrl,mimeType,mediaMetadata,filename),nextPageToken",
			"pageSize":  "100",
			"pageToken": "first",
		},
		http.MethodGet)
}

func (d *GooglePhoto) getMedia(id string) (MediaItem, error) {
	var resp MediaItem

	query := map[string]string{
		"fields": "mediaMetadata,baseUrl,mimeType",
	}
	_, err := d.request(fmt.Sprintf("https://photoslibrary.googleapis.com/v1/mediaItems/%s", id), http.MethodGet, func(req *resty.Request) {
		req.SetQueryParams(query)
	}, &resp, nil)
	if err != nil {
		return resp, err
	}

	return resp, nil
}

func (d *GooglePhoto) fetchItems(url string, query map[string]string, method string) ([]MediaItem, error){
	res := make([]MediaItem, 0)
	for query["pageToken"] != "" {
		if query["pageToken"] == "first" {
			query["pageToken"] = ""
		}
		var resp Items

		_, err := d.request(url, method, func(req *resty.Request) {
			req.SetQueryParams(query)
		}, &resp, nil)
		if err != nil {
			return nil, err
		}
		query["pageToken"] = resp.NextPageToken
		res = append(res, resp.MediaItems...)
		res = append(res, resp.Albums...)
		res = append(res, resp.SharedAlbums...)
	}
	return res, nil
}
