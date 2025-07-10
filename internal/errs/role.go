package errs

import "errors"

var (
	ErrChangeDefaultRole = errors.New("cannot modify admin or guest role")
)
