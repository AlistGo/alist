package errs

import "errors"

var (
	TooManyDevices  = errors.New("too many active devices")
	SessionInactive = errors.New("session inactive")
)
