package model

import (
	"fmt"
	"time"
)

type Size int64

func formatSize(size int64) string {
	if size < 0 {
		return "Unknown"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	index := 0
	fsize := float64(size)
	for fsize > 1024 && index < len(units)-1 {
		fsize /= 1024
		index++
	}
	return fmt.Sprintf("%.2f %s", fsize, units[index])
}

func (s Size) String() string {
	return formatSize(int64(s))
}

// Usage 存储使用量信息
type Usage struct {
	Available int64 `json:"available"`
	Used      int64 `json:"used"`
	Total     int64 `json:"total"`
}

// NewEmptyUsage 创建一个新的未知使用量信息（无限容量）
func NewEmptyUsage() *Usage {
	return &Usage{
		Available: -1,
		Used:      0,
		Total:     -1,
	}
}

// SetTotalGB 设置总容量（GB单位）
func (u *Usage) SetTotalGB(totalGB int64) {
	if totalGB <= 0 {
		u.Total = -1
		u.Available = -1
		return
	}
	u.Total = totalGB * 1024 * 1024 * 1024
	u.Available = u.Total - u.Used
	if u.Available < 0 {
		u.Available = 0
	}
}

// AddUsed 增加已使用容量
func (u *Usage) AddUsed(size int64) {
	u.Used += size
	if u.Total > 0 {
		u.Available = u.Total - u.Used
		if u.Available < 0 {
			u.Available = 0
		}
	}
}

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

type MountedStorage struct {
}
