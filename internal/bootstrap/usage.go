package bootstrap

import (
	"github.com/alist-org/alist/v3/internal/usage"
	log "github.com/sirupsen/logrus"
)

// InitUsage 初始化使用量统计
func InitUsage() {
	log.Info("init usage calculation")
	usage.Init()
}
