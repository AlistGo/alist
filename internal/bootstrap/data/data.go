package data

import "github.com/alist-org/alist/v3/cmd/flags"

func InitData() {
	initRoles()
	initUser()
	initSettings()
	initTasks()
	if flags.Dev {
		initDevData()
		initDevDo()
	}
}
