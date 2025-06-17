package data

import (
	"os"

	"github.com/alist-org/alist/v3/cmd/flags"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func initUser() {
	admin, err := op.GetAdmin()
	adminPassword := random.String(8)
	envpass := os.Getenv("ALIST_ADMIN_PASSWORD")
	if flags.Dev {
		adminPassword = "admin"
	} else if len(envpass) > 0 {
		adminPassword = envpass
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			salt := random.String(16)
			admin = &model.User{
				Username: "admin",
				Salt:     salt,
				PwdHash:  model.TwoHashPwd(adminPassword, salt),
				Role:     model.ADMIN,
				BasePath: "/",
				Authn:    "[]",
				// 0(can see hidden) - 7(can remove) & 12(can read archives) - 13(can decompress archives)
				Permission: 0x30FF,
			}
			if err := op.CreateUser(admin); err != nil {
				panic(err)
			} else {
				utils.Log.Infof("Successfully created the admin user and the initial password is: %s", adminPassword)
			}
		} else {
			utils.Log.Fatalf("[init user] Failed to get admin user: %v", err)
		}
	}
	guest, err := op.GetGuest()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			salt := random.String(16)
			guest = &model.User{
				Username:   "guest",
				PwdHash:    model.TwoHashPwd("guest", salt),
				Salt:       salt,
				Role:       model.GUEST,
				BasePath:   "/",
				Permission: 0,
				Disabled:   true,
				Authn:      "[]",
			}
			if err := db.CreateUser(guest); err != nil {
				utils.Log.Fatalf("[init user] Failed to create guest user: %v", err)
			}
		} else {
			utils.Log.Fatalf("[init user] Failed to get guest user: %v", err)
		}
	}
}
