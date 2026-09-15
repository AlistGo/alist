package v3_24_0

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/op"
)

// HashPwdForOldVersion encode passwords using SHA256
// First published: 75acbcc perf: sha256 for user's password (close #3552) by Andy Hsu
func HashPwdForOldVersion() error {
	users, _, err := op.GetUsers(1, -1)
	if err != nil {
		return fmt.Errorf("hash passwords: get users: %w", err)
	}
	for i := range users {
		user := users[i]
		if user.PwdHash == "" {
			user.SetPassword(user.Password)
			user.Password = ""
			if err := db.UpdateUser(&user); err != nil {
				return fmt.Errorf("hash passwords: update user: %w", err)
			}
		}
	}
	return nil
}
