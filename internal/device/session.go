package device

import (
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// Handle verifies device sessions for a user and upserts current session.
func Handle(userID uint, deviceKey, ua, ip string) error {
	ttl := setting.GetInt(conf.DeviceSessionTTL, 86400)
	if ttl > 0 {
		_ = db.DeleteSessionsBefore(time.Now().Unix() - int64(ttl))
	}

	ip = utils.MaskIP(ip)

	now := time.Now().Unix()
	sess, err := db.GetSession(userID, deviceKey)
	if err == nil {
		if sess.Status == model.SessionInactive {
			return errors.WithStack(errs.SessionInactive)
		}
		sess.Status = model.SessionActive
		sess.LastActive = now
		sess.UserAgent = ua
		sess.IP = ip
		return db.UpsertSession(sess)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	max := setting.GetInt(conf.MaxDevices, 0)
	if max > 0 {
		count, err := db.CountActiveSessionsByUser(userID)
		if err != nil {
			return err
		}
		if count >= int64(max) {
			policy := setting.GetStr(conf.DeviceEvictPolicy, "deny")
			if policy == "evict_oldest" {
				if oldest, err := db.GetOldestActiveSession(userID); err == nil {
					if err := db.MarkInactive(oldest.DeviceKey); err != nil {
						return err
					}
				}
			} else {
				return errors.WithStack(errs.TooManyDevices)
			}
		}
	}

	s := &model.Session{UserID: userID, DeviceKey: deviceKey, UserAgent: ua, IP: ip, LastActive: now, Status: model.SessionActive}
	return db.CreateSession(s)
}

// EnsureActiveOnLogin is used only in login flow:
// - If session exists (even Inactive): reactivate and refresh fields.
// - If not exists: apply max-devices policy, then create Active session.
func EnsureActiveOnLogin(userID uint, deviceKey, ua, ip string) error {
	ip = utils.MaskIP(ip)
	now := time.Now().Unix()

	sess, err := db.GetSession(userID, deviceKey)
	if err == nil {
		if sess.Status == model.SessionInactive {
			max := setting.GetInt(conf.MaxDevices, 0)
			if max > 0 {
				count, err := db.CountActiveSessionsByUser(userID)
				if err != nil {
					return err
				}
				if count >= int64(max) {
					policy := setting.GetStr(conf.DeviceEvictPolicy, "deny")
					if policy == "evict_oldest" {
						if oldest, gerr := db.GetOldestActiveSession(userID); gerr == nil {
							if err := db.MarkInactive(oldest.DeviceKey); err != nil {
								return err
							}
						}
					} else {
						return errors.WithStack(errs.TooManyDevices)
					}
				}
			}
		}
		sess.Status = model.SessionActive
		sess.LastActive = now
		sess.UserAgent = ua
		sess.IP = ip
		return db.UpsertSession(sess)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	max := setting.GetInt(conf.MaxDevices, 0)
	if max > 0 {
		count, err := db.CountActiveSessionsByUser(userID)
		if err != nil {
			return err
		}
		if count >= int64(max) {
			policy := setting.GetStr(conf.DeviceEvictPolicy, "deny")
			if policy == "evict_oldest" {
				if oldest, gerr := db.GetOldestActiveSession(userID); gerr == nil {
					if err := db.MarkInactive(oldest.DeviceKey); err != nil {
						return err
					}
				}
			} else {
				return errors.WithStack(errs.TooManyDevices)
			}
		}
	}

	return db.CreateSession(&model.Session{
		UserID:     userID,
		DeviceKey:  deviceKey,
		UserAgent:  ua,
		IP:         ip,
		LastActive: now,
		Status:     model.SessionActive,
	})
}

// Refresh updates last_active for the session.
func Refresh(userID uint, deviceKey string) {
	_ = db.UpdateSessionLastActive(userID, deviceKey, time.Now().Unix())
}
