package handles

import (
	"context"
	"strconv"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func ListStorages(c *gin.Context) {
	var req model.PageReq
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	req.Validate()
	log.Debugf("%+v", req)
	storages, total, err := db.GetStorages(req.Page, req.PerPage)
	if err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c, common.PageResp{
		Content: storages,
		Total:   total,
	})
}

func CreateStorage(c *gin.Context) {
	var req model.Storage
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if id, err := op.CreateStorage(c, req); err != nil {
		common.ErrorWithDataResp(c, err, 500, gin.H{
			"id": id,
		}, true)
	} else {
		common.SuccessResp(c, gin.H{
			"id": id,
		})
	}
}

func UpdateStorage(c *gin.Context) {
	var req model.Storage
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.UpdateStorage(c, req); err != nil {
		common.ErrorResp(c, err, 500, true)
	} else {
		common.SuccessResp(c)
	}
}

func DeleteStorage(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.DeleteStorageById(c, uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func DisableStorage(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.DisableStorage(c, uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func EnableStorage(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	if err := op.EnableStorage(c, uint(id)); err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c)
}

func GetStorage(c *gin.Context) {
	idStr := c.Query("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	storage, err := db.GetStorageById(uint(id))
	if err != nil {
		common.ErrorResp(c, err, 500, true)
		return
	}
	common.SuccessResp(c, storage)
}

func LoadAllStorages(c *gin.Context) {
	storages, err := db.GetEnabledStorages()
	if err != nil {
		log.Errorf("failed get enabled storages: %+v", err)
		common.ErrorResp(c, err, 500, true)
		return
	}
	conf.StoragesLoaded.Store(false)
	go func(storages []model.Storage) {
		for _, storage := range storages {
			storageDriver, err := op.GetStorageByMountPath(storage.MountPath)
			if err != nil {
				log.Errorf("failed get storage driver: %+v", err)
				continue
			}
			// drop the storage in the driver
			if err := storageDriver.Drop(context.Background()); err != nil {
				log.Errorf("failed drop storage: %+v", err)
				continue
			}
			if err := op.LoadStorage(context.Background(), storage); err != nil {
				log.Errorf("failed get enabled storages: %+v", err)
				continue
			}
			log.Infof("success load storage: [%s], driver: [%s]",
				storage.MountPath, storage.Driver)
		}
		conf.StoragesLoaded.Store(true)
	}(storages)
	common.SuccessResp(c)
}
