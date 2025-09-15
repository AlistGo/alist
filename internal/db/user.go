package db

import (
	"encoding/base64"
	"fmt"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"path"
	"slices"
	"strings"
)

func GetUserByRole(role int) (*model.User, error) {
	var users []model.User
	if err := db.Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].Role.Contains(role) {
			return &users[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func GetUsersByRole(roleID int) ([]model.User, error) {
	var users []model.User
	if err := db.Find(&users).Error; err != nil {
		return nil, err
	}
	var result []model.User
	for _, u := range users {
		if slices.Contains(u.Role, roleID) {
			result = append(result, u)
		}
	}
	return result, nil
}

func GetUserByName(username string) (*model.User, error) {
	user := model.User{Username: username}
	if err := db.Where(user).First(&user).Error; err != nil {
		return nil, errors.Wrapf(err, "failed find user")
	}
	return &user, nil
}

func GetUserBySSOID(ssoID string) (*model.User, error) {
	user := model.User{SsoID: ssoID}
	if err := db.Where(user).First(&user).Error; err != nil {
		return nil, errors.Wrapf(err, "The single sign on platform is not bound to any users")
	}
	return &user, nil
}

func GetUserById(id uint) (*model.User, error) {
	var u model.User
	if err := db.First(&u, id).Error; err != nil {
		return nil, errors.Wrapf(err, "failed get old user")
	}
	return &u, nil
}

func CreateUser(u *model.User) error {
	return errors.WithStack(db.Create(u).Error)
}

func UpdateUser(u *model.User) error {
	return errors.WithStack(db.Save(u).Error)
}

func GetUsers(pageIndex, pageSize int) (users []model.User, count int64, err error) {
	userDB := db.Model(&model.User{})
	if err := userDB.Count(&count).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get users count")
	}
	if err := userDB.Order(columnName("id")).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "failed get find users")
	}
	return users, count, nil
}

func GetAllUsers() ([]model.User, error) {
	var users []model.User
	if err := db.Find(&users).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return users, nil
}

func DeleteUserById(id uint) error {
	return errors.WithStack(db.Delete(&model.User{}, id).Error)
}

func UpdateAuthn(userID uint, authn string) error {
	return db.Model(&model.User{ID: userID}).Update("authn", authn).Error
}

func RegisterAuthn(u *model.User, credential *webauthn.Credential) error {
	if u == nil {
		return errors.New("user is nil")
	}
	exists := u.WebAuthnCredentials()
	if credential != nil {
		exists = append(exists, *credential)
	}
	res, err := utils.Json.Marshal(exists)
	if err != nil {
		return err
	}
	return UpdateAuthn(u.ID, string(res))
}

func RemoveAuthn(u *model.User, id string) error {
	exists := u.WebAuthnCredentials()
	for i := 0; i < len(exists); i++ {
		idEncoded := base64.StdEncoding.EncodeToString(exists[i].ID)
		if idEncoded == id {
			exists[len(exists)-1], exists[i] = exists[i], exists[len(exists)-1]
			exists = exists[:len(exists)-1]
			break
		}
	}

	res, err := utils.Json.Marshal(exists)
	if err != nil {
		return err
	}
	return UpdateAuthn(u.ID, string(res))
}

func UpdateUserBasePathPrefix(oldPath, newPath string, usersOpt ...[]model.User) ([]string, error) {
	var users []model.User
	var modifiedUsernames []string

	oldPathClean := path.Clean(oldPath)

	if len(usersOpt) > 0 {
		users = usersOpt[0]
	} else {
		if err := db.Find(&users).Error; err != nil {
			return nil, errors.WithMessage(err, "failed to load users")
		}
	}

	for _, user := range users {
		basePath := path.Clean(user.BasePath)
		updated := false

		if basePath == oldPathClean {
			user.BasePath = path.Clean(newPath)
			updated = true
		} else if strings.HasPrefix(basePath, oldPathClean+"/") {
			user.BasePath = path.Clean(newPath + basePath[len(oldPathClean):])
			updated = true
		}

		if updated {
			if err := UpdateUser(&user); err != nil {
				return nil, errors.WithMessagef(err, "failed to update user ID %d", user.ID)
			}
			modifiedUsernames = append(modifiedUsernames, user.Username)
		}
	}

	return modifiedUsernames, nil
}

func CountUsersByRoleAndEnabledExclude(roleID uint, excludeUserID uint) (int64, error) {
	var count int64
	jsonValue := fmt.Sprintf("[%d]", roleID)
	err := db.Model(&model.User{}).
		Where("disabled = ? AND id != ?", false, excludeUserID).
		Where("JSON_CONTAINS(role, ?)", jsonValue).
		Count(&count).Error
	return count, err
}
