package usage

import (
	"context"
	"sync"
	"time"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/search"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/alist-org/alist/v3/pkg/utils"
	log "github.com/sirupsen/logrus"
)

var (
	globalUsage  = model.NewEmptyUsage()
	usageLock    = &sync.RWMutex{}
	lastScanTime = time.Time{}
	isScanning   = false
	scanningLock = &sync.Mutex{}
)

// GetGlobalUsage 获取全局使用量信息
func GetGlobalUsage() *model.Usage {
	usageLock.RLock()
	defer usageLock.RUnlock()
	return globalUsage
}

// GetGlobalStorageSizeGB 获取全局存储容量（GB）
func GetGlobalStorageSizeGB() int64 {
	sizeStr := setting.GetStr(conf.GlobalStorageSize)
	size, err := utils.ParseInt(sizeStr)
	if err != nil || size <= -2 {
		// 如果解析失败或值非法，使用默认值 -1
		log.Warnf("invalid global storage size: %s, using default -1", sizeStr)
		return -1
	}
	return size
}

// GetScanInterval 获取扫描间隔（秒）
func GetScanInterval() int64 {
	intervalStr := setting.GetStr(conf.UsageScanInterval)
	interval, err := utils.ParseInt(intervalStr)
	if err != nil || interval < 0 {
		// 如果解析失败或值为负数，使用默认值 3600
		log.Warnf("invalid scan interval: %s, using default 3600", intervalStr)
		return 3600
	}
	return interval
}

// ScanUsageIfNeeded 如果需要则扫描使用量
func ScanUsageIfNeeded() {
	// 如果全局容量设置为 -1 或 0，则不扫描
	globalSize := GetGlobalStorageSizeGB()
	if globalSize <= 0 {
		return
	}

	// 检查是否超过扫描间隔
	scanInterval := GetScanInterval()
	now := time.Now()

	scanningLock.Lock()
	if isScanning || now.Sub(lastScanTime).Seconds() < float64(scanInterval) {
		scanningLock.Unlock()
		return
	}
	isScanning = true
	scanningLock.Unlock()

	go func() {
		defer func() {
			scanningLock.Lock()
			isScanning = false
			lastScanTime = time.Now()
			scanningLock.Unlock()
		}()

		log.Infof("start scanning usage")
		ctx := context.Background()
		size, err := CalculateUsage(ctx)
		if err != nil {
			log.Errorf("calculate usage error: %+v", err)
			return
		}

		usageLock.Lock()
		globalUsage.Used = size
		globalUsage.SetTotalGB(globalSize)
		usageLock.Unlock()

		log.Infof("usage scan completed: used %s, total %s",
			model.Size(size).String(),
			model.Size(globalUsage.Total).String())
	}()
}

// CalculateUsage 计算所有存储的使用量
func CalculateUsage(ctx context.Context) (int64, error) {
	// 使用搜索来计算总大小
	var totalSize int64 = 0

	// 获取所有文件信息
	nodes, err := search.GetAllNodes(ctx)
	if err != nil {
		return 0, err
	}

	// 累计文件大小
	for _, node := range nodes {
		if !node.IsDir {
			totalSize += node.Size
		}
	}

	return totalSize, nil
}

// Init 初始化使用量模块
func Init() {
	globalSize := GetGlobalStorageSizeGB()
	usageLock.Lock()
	globalUsage.SetTotalGB(globalSize)
	usageLock.Unlock()

	// 初始进行一次扫描
	if globalSize > 0 {
		go ScanUsageIfNeeded()
	}
}
