package v3_32_0

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/op"
)

// UpdateAuthnForOldVersion updates users' authn
// First published: bdfc159 fix: webauthn logspam (#6181) by itsHenry
func UpdateAuthnForOldVersion() error {
	users, _, err := op.GetUsers(1, -1)
	if err != nil {
		return fmt.Errorf("update authn: get users: %w", err)
	}
	for i := range users {
		user := users[i]
		if user.Authn == "" {
			user.Authn = "[]"
			if err := db.UpdateUser(&user); err != nil {
				return fmt.Errorf("update authn: update user: %w", err)
			}
		}
	}
	return nil
}
