package session

import "github.com/alist-org/alist/v3/internal/db"

// MarkInactive marks the session with the given ID as inactive.
func MarkInactive(sessionID string) error {
	return db.MarkInactive(sessionID)
}
