package model

import (
	"time"
)

type Storage struct {
	ID              uint      `json:"id" gorm:"primaryKey"`                        // unique key
	MountPath       string    `json:"mount_path" gorm:"unique" binding:"required"` // must be standardized
	Order           int       `json:"order"`                                       // use to sort
	Driver          string    `json:"driver"`                                      // driver used
	CacheExpiration int       `json:"cache_expiration"`                            // cache expire time
	Status          string    `json:"status"`
	Addition        string    `json:"addition" gorm:"type:text"` // Additional information, defined in the corresponding driver
	Remark          string    `json:"remark"`
	Modified        time.Time `json:"modified"`
	Disabled        bool      `json:"disabled"` // if disabled
	DisableIndex    bool      `json:"disable_index"`
	EnableSign      bool      `json:"enable_sign"`
	Sort
	Proxy
}

type Sort struct {
	OrderBy        string `json:"order_by"`
	OrderDirection string `json:"order_direction"`
	ExtractFolder  string `json:"extract_folder"`
}

type Proxy struct {
	WebProxy     bool   `json:"web_proxy"`
	WebdavPolicy string `json:"webdav_policy"`
	ProxyRange   bool   `json:"proxy_range"`
	DownProxyUrl string `json:"down_proxy_url"`
}

func (s *Storage) GetStorage() *Storage {
	return s
}

func (s *Storage) SetStorage(storage Storage) {
	*s = storage
}

func (s *Storage) SetStatus(status string) {
	s.Status = status
}

func (p Proxy) Webdav302() bool {
	return p.WebdavPolicy == "302_redirect"
}

func (p Proxy) WebdavProxy() bool {
	return p.WebdavPolicy == "use_proxy_url"
}

func (p Proxy) WebdavNative() bool {
	return !p.Webdav302() && !p.WebdavProxy()
}
