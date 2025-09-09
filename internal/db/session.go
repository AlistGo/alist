package db

import (
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"gorm.io/gorm/clause"
)

type SessionWithUser struct {
	model.Session
	Username string
}

func GetSession(userID uint, deviceKey string) (*model.Session, error) {
	s := model.Session{UserID: userID, DeviceKey: deviceKey}
	if err := db.Select("user_id, device_key, last_active, status, user_agent, ip").Where(&s).First(&s).Error; err != nil {
		return nil, errors.Wrap(err, "failed find session")
	}
	return &s, nil
}

func CreateSession(s *model.Session) error {
	return errors.WithStack(db.Create(s).Error)
}

func UpsertSession(s *model.Session) error {
	return errors.WithStack(db.Clauses(clause.OnConflict{UpdateAll: true}).Create(s).Error)
}

func DeleteSession(userID uint, deviceKey string) error {
	return errors.WithStack(db.Where("user_id = ? AND device_key = ?", userID, deviceKey).Delete(&model.Session{}).Error)
}

func CountActiveSessionsByUser(userID uint) (int64, error) {
	var count int64
	err := db.Model(&model.Session{}).
		Where("user_id = ? AND status = ?", userID, model.SessionActive).
		Count(&count).Error
	return count, errors.WithStack(err)
}

func DeleteSessionsBefore(ts int64) error {
	return errors.WithStack(db.Where("last_active < ?", ts).Delete(&model.Session{}).Error)
}

// GetOldestActiveSession returns the oldest active session for the specified user.
func GetOldestActiveSession(userID uint) (*model.Session, error) {
	var s model.Session
	if err := db.Where("user_id = ? AND status = ?", userID, model.SessionActive).
		Order("last_active ASC").First(&s).Error; err != nil {
		return nil, errors.Wrap(err, "failed get oldest active session")
	}
	return &s, nil
}

func UpdateSessionLastActive(userID uint, deviceKey string, lastActive int64) error {
	return errors.WithStack(db.Model(&model.Session{}).Where("user_id = ? AND device_key = ?", userID, deviceKey).Update("last_active", lastActive).Error)
}

func ListSessionsByUser(userID uint) ([]model.Session, error) {
	var sessions []model.Session
	err := db.Select("user_id, device_key, last_active, status, user_agent, ip").Where("user_id = ? AND status = ?", userID, model.SessionActive).Find(&sessions).Error
	return sessions, errors.WithStack(err)
}

func ListSessions() ([]model.Session, error) {
	var sessions []model.Session
	err := db.Select("user_id, device_key, last_active, status, user_agent, ip").Where("status = ?", model.SessionActive).Find(&sessions).Error
	return sessions, errors.WithStack(err)
}

func ListSessionsWithUser() ([]SessionWithUser, error) {
	var sessions []SessionWithUser
	sessionTable := conf.Conf.Database.TablePrefix + "sessions"
	userTable := conf.Conf.Database.TablePrefix + "users"
	err := db.Table(sessionTable).
		Select(sessionTable+".user_id, "+sessionTable+".device_key, "+sessionTable+".last_active, "+
			sessionTable+".status, "+sessionTable+".user_agent, "+sessionTable+".ip, "+userTable+".username").
		Joins("JOIN "+userTable+" ON "+sessionTable+".user_id = "+userTable+".id").
		Where(sessionTable+".status = ?", model.SessionActive).
		Scan(&sessions).Error
	return sessions, errors.WithStack(err)
}

func MarkInactive(sessionID string) error {
	return errors.WithStack(db.Model(&model.Session{}).Where("device_key = ?", sessionID).Update("status", model.SessionInactive).Error)
}

func DeleteInactiveSessions(userID *uint) error {
	query := db.Where("status = ?", model.SessionInactive)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	return errors.WithStack(query.Delete(&model.Session{}).Error)
}

func DeleteSessionByID(sessionID string) error {
	return errors.WithStack(db.Where("device_key = ?", sessionID).Delete(&model.Session{}).Error)
}
