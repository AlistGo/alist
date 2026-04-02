package share

import (
	"fmt"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/setting"
	signPkg "github.com/alist-org/alist/v3/pkg/sign"
)

func tokenPayload(share *model.Share) string {
	updatedAt := int64(0)
	if !share.UpdatedAt.IsZero() {
		updatedAt = share.UpdatedAt.Unix()
	}
	return fmt.Sprintf("%s:%s:%d", share.ShareID, share.PasswordHash, updatedAt)
}

func signer() signPkg.Sign {
	return signPkg.NewHMACSign([]byte(setting.GetStr(conf.Token) + "-share-access"))
}

func SignAccess(share *model.Share, d time.Duration) string {
	return signer().Sign(tokenPayload(share), time.Now().Add(d).Unix())
}

func VerifyAccess(share *model.Share, token string) error {
	return signer().Verify(tokenPayload(share), token)
}
