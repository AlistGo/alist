package emby

import "time"

type AuthResp struct {
	User struct {
		Id     string `json:"Id"`
		Policy struct {
			EnableContentDownloading bool `json:"EnableContentDownloading"`
		} `json:"Policy"`
	} `json:"User"`
	AccessToken string `json:"AccessToken"`
}

type MediaSource struct {
	Path string `json:"Path"`
	Size int64  `json:"Size"`
}

type Item struct {
	Id           string            `json:"Id"`
	Name         string            `json:"Name"`
	Type         string            `json:"Type"`
	MediaType    string            `json:"MediaType"`
	IsFolder     bool              `json:"IsFolder"`
	Path         string            `json:"Path"`
	Container    string            `json:"Container"`
	Size         int64             `json:"Size"`
	DateCreated  time.Time         `json:"DateCreated"`
	MediaSources []MediaSource     `json:"MediaSources"`
	ImageTags    map[string]string `json:"ImageTags"`
}

type ItemsResp struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}
