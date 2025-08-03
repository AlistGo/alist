package errs

import "errors"

var (
	EmptyToken = errors.New("empty token")
	LinkIsDir  = errors.New("link is dir")
)
