package bootstrap

import (
	"context"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/internal/stream"
	"golang.org/x/time/rate"
)

type blockBurstLimiter struct {
	*rate.Limiter
}

func (l blockBurstLimiter) WaitN(ctx context.Context, total int) error {
	for total > 0 {
		n := l.Burst()
		if l.Limiter.Limit() == rate.Inf || n > total {
			n = total
		}
		err := l.Limiter.WaitN(ctx, n)
		if err != nil {
			return err
		}
		total -= n
	}
	return nil
}

func streamFilterNegative(limit int) (rate.Limit, int) {
	if limit < 0 {
		return rate.Inf, 0
	}
	return rate.Limit(limit) * 1024.0, limit * 1024
}

func initLimiter(limiter *stream.Limiter, s string) {
	clientDownLimit, burst := streamFilterNegative(setting.GetInt(s, -1))
	*limiter = blockBurstLimiter{Limiter: rate.NewLimiter(clientDownLimit, burst)}
	op.RegisterSettingChangingCallback(func() {
		newLimit, newBurst := streamFilterNegative(setting.GetInt(s, -1))
		(*limiter).SetLimit(newLimit)
		(*limiter).SetBurst(newBurst)
	})
}

func InitStreamLimit() {
	initLimiter(&stream.ClientDownloadLimit, conf.StreamMaxClientDownloadSpeed)
	initLimiter(&stream.ClientUploadLimit, conf.StreamMaxClientUploadSpeed)
	initLimiter(&stream.ServerDownloadLimit, conf.StreamMaxServerDownloadSpeed)
	initLimiter(&stream.ServerUploadLimit, conf.StreamMaxServerUploadSpeed)
}
