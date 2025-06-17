package bootstrap

import (
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/fs"
	"github.com/alist-org/alist/v3/internal/offline_download/tool"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/setting"
	"github.com/xhofe/tache"
)

func taskFilterNegative(num int) int64 {
	if num < 0 {
		num = 0
	}
	return int64(num)
}

func InitTaskManager() {
	fs.UploadTaskManager = tache.NewManager[*fs.UploadTask](tache.WithWorks(setting.GetInt(conf.TaskUploadThreadsNum, conf.Conf.Tasks.Upload.Workers)), tache.WithMaxRetry(conf.Conf.Tasks.Upload.MaxRetry)) //upload will not support persist
	op.RegisterSettingChangingCallback(func() {
		fs.UploadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskUploadThreadsNum, conf.Conf.Tasks.Upload.Workers)))
	})
	fs.CopyTaskManager = tache.NewManager[*fs.CopyTask](tache.WithWorks(setting.GetInt(conf.TaskCopyThreadsNum, conf.Conf.Tasks.Copy.Workers)), tache.WithPersistFunction(db.GetTaskDataFunc("copy", conf.Conf.Tasks.Copy.TaskPersistant), db.UpdateTaskDataFunc("copy", conf.Conf.Tasks.Copy.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Copy.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.CopyTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskCopyThreadsNum, conf.Conf.Tasks.Copy.Workers)))
	})
	tool.DownloadTaskManager = tache.NewManager[*tool.DownloadTask](tache.WithWorks(setting.GetInt(conf.TaskOfflineDownloadThreadsNum, conf.Conf.Tasks.Download.Workers)), tache.WithPersistFunction(db.GetTaskDataFunc("download", conf.Conf.Tasks.Download.TaskPersistant), db.UpdateTaskDataFunc("download", conf.Conf.Tasks.Download.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Download.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		tool.DownloadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskOfflineDownloadThreadsNum, conf.Conf.Tasks.Download.Workers)))
	})
	tool.TransferTaskManager = tache.NewManager[*tool.TransferTask](tache.WithWorks(setting.GetInt(conf.TaskOfflineDownloadTransferThreadsNum, conf.Conf.Tasks.Transfer.Workers)), tache.WithPersistFunction(db.GetTaskDataFunc("transfer", conf.Conf.Tasks.Transfer.TaskPersistant), db.UpdateTaskDataFunc("transfer", conf.Conf.Tasks.Transfer.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Transfer.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		tool.TransferTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskOfflineDownloadTransferThreadsNum, conf.Conf.Tasks.Transfer.Workers)))
	})
	if len(tool.TransferTaskManager.GetAll()) == 0 { //prevent offline downloaded files from being deleted
		CleanTempDir()
	}
	fs.ArchiveDownloadTaskManager = tache.NewManager[*fs.ArchiveDownloadTask](tache.WithWorks(setting.GetInt(conf.TaskDecompressDownloadThreadsNum, conf.Conf.Tasks.Decompress.Workers)), tache.WithPersistFunction(db.GetTaskDataFunc("decompress", conf.Conf.Tasks.Decompress.TaskPersistant), db.UpdateTaskDataFunc("decompress", conf.Conf.Tasks.Decompress.TaskPersistant)), tache.WithMaxRetry(conf.Conf.Tasks.Decompress.MaxRetry))
	op.RegisterSettingChangingCallback(func() {
		fs.ArchiveDownloadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskDecompressDownloadThreadsNum, conf.Conf.Tasks.Decompress.Workers)))
	})
	fs.ArchiveContentUploadTaskManager.Manager = tache.NewManager[*fs.ArchiveContentUploadTask](tache.WithWorks(setting.GetInt(conf.TaskDecompressUploadThreadsNum, conf.Conf.Tasks.DecompressUpload.Workers)), tache.WithMaxRetry(conf.Conf.Tasks.DecompressUpload.MaxRetry)) //decompress upload will not support persist
	op.RegisterSettingChangingCallback(func() {
		fs.ArchiveContentUploadTaskManager.SetWorkersNumActive(taskFilterNegative(setting.GetInt(conf.TaskDecompressUploadThreadsNum, conf.Conf.Tasks.DecompressUpload.Workers)))
	})
}
