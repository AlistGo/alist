package errs

import "errors"

var (
	ErrChangeDefaultRole = errors.New("cannot modify admin role")
)
