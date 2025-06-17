package op

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/generic_sync"
	"github.com/alist-org/alist/v3/pkg/utils"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Although the driver type is stored,
// there is a storage in each driver,
// so it should actually be a storage, just wrapped by the driver
var storagesMap generic_sync.MapOf[string, driver.Driver]

func GetAllStorages() []driver.Driver {
	return storagesMap.Values()
}

func HasStorage(mountPath string) bool {
	return storagesMap.Has(utils.FixAndCleanPath(mountPath))
}

func GetStorageByMountPath(mountPath string) (driver.Driver, error) {
	mountPath = utils.FixAndCleanPath(mountPath)
	storageDriver, ok := storagesMap.Load(mountPath)
	if !ok {
		return nil, errors.Errorf("no mount path for an storage is: %s", mountPath)
	}
	return storageDriver, nil
}

// CreateStorage Save the storage to database so storage can get an id
// then instantiate corresponding driver and save it in memory
func CreateStorage(ctx context.Context, storage model.Storage) (uint, error) {
	storage.Modified = time.Now()
	storage.MountPath = utils.FixAndCleanPath(storage.MountPath)
	var err error
	// check driver first
	driverName := storage.Driver
	driverNew, err := GetDriver(driverName)
	if err != nil {
		return 0, errors.WithMessage(err, "failed get driver new")
	}
	storageDriver := driverNew()
	// insert storage to database
	err = db.CreateStorage(&storage)
	if err != nil {
		return storage.ID, errors.WithMessage(err, "failed create storage in database")
	}
	// already has an id
	err = initStorage(ctx, storage, storageDriver)
	go callStorageHooks("add", storageDriver)
	if err != nil {
		return storage.ID, errors.Wrap(err, "failed init storage but storage is already created")
	}
	log.Debugf("storage %+v is created", storageDriver)
	return storage.ID, nil
}

// LoadStorage load exist storage in db to memory
func LoadStorage(ctx context.Context, storage model.Storage) error {
	storage.MountPath = utils.FixAndCleanPath(storage.MountPath)
	// check driver first
	driverName := storage.Driver
	driverNew, err := GetDriver(driverName)
	if err != nil {
		return errors.WithMessage(err, "failed get driver new")
	}
	storageDriver := driverNew()

	err = initStorage(ctx, storage, storageDriver)
	go callStorageHooks("add", storageDriver)
	log.Debugf("storage %+v is created", storageDriver)
	return err
}

func getCurrentGoroutineStack() string {
	buf := make([]byte, 1<<16)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// initStorage initialize the driver and store to storagesMap
func initStorage(ctx context.Context, storage model.Storage, storageDriver driver.Driver) (err error) {
	storageDriver.SetStorage(storage)
	driverStorage := storageDriver.GetStorage()
	defer func() {
		if err := recover(); err != nil {
			errInfo := fmt.Sprintf("[panic] err: %v\nstack: %s\n", err, getCurrentGoroutineStack())
			log.Errorf("panic init storage: %s", errInfo)
			driverStorage.SetStatus(errInfo)
			MustSaveDriverStorage(storageDriver)
			storagesMap.Store(driverStorage.MountPath, storageDriver)
		}
	}()
	// Unmarshal Addition
	err = utils.Json.UnmarshalFromString(driverStorage.Addition, storageDriver.GetAddition())
	if err == nil {
		if ref, ok := storageDriver.(driver.Reference); ok {
			if strings.HasPrefix(driverStorage.Remark, "ref:/") {
				refMountPath := driverStorage.Remark
				i := strings.Index(refMountPath, "\n")
				if i > 0 {
					refMountPath = refMountPath[4:i]
				} else {
					refMountPath = refMountPath[4:]
				}
				var refStorage driver.Driver
				refStorage, err = GetStorageByMountPath(refMountPath)
				if err != nil {
					err = fmt.Errorf("ref: %w", err)
				} else {
					err = ref.InitReference(refStorage)
					if err != nil && errs.IsNotSupportError(err) {
						err = fmt.Errorf("ref: storage is not %s", storageDriver.Config().Name)
					}
				}
			}
		}
	}
	if err == nil {
		err = storageDriver.Init(ctx)
	}
	storagesMap.Store(driverStorage.MountPath, storageDriver)
	if err != nil {
		driverStorage.SetStatus(err.Error())
		err = errors.Wrap(err, "failed init storage")
	} else {
		driverStorage.SetStatus(WORK)
	}
	MustSaveDriverStorage(storageDriver)
	return err
}

func EnableStorage(ctx context.Context, id uint) error {
	storage, err := db.GetStorageById(id)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	if !storage.Disabled {
		return errors.Errorf("this storage have enabled")
	}
	storage.Disabled = false
	err = db.UpdateStorage(storage)
	if err != nil {
		return errors.WithMessage(err, "failed update storage in db")
	}
	err = LoadStorage(ctx, *storage)
	if err != nil {
		return errors.WithMessage(err, "failed load storage")
	}
	return nil
}

func DisableStorage(ctx context.Context, id uint) error {
	storage, err := db.GetStorageById(id)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	if storage.Disabled {
		return errors.Errorf("this storage have disabled")
	}
	storageDriver, err := GetStorageByMountPath(storage.MountPath)
	if err != nil {
		return errors.WithMessage(err, "failed get storage driver")
	}
	// drop the storage in the driver
	if err := storageDriver.Drop(ctx); err != nil {
		return errors.Wrap(err, "failed drop storage")
	}
	// delete the storage in the memory
	storage.Disabled = true
	storage.SetStatus(DISABLED)
	err = db.UpdateStorage(storage)
	if err != nil {
		return errors.WithMessage(err, "failed update storage in db")
	}
	storagesMap.Delete(storage.MountPath)
	go callStorageHooks("del", storageDriver)
	return nil
}

// UpdateStorage update storage
// get old storage first
// drop the storage then reinitialize
func UpdateStorage(ctx context.Context, storage model.Storage) error {
	oldStorage, err := db.GetStorageById(storage.ID)
	if err != nil {
		return errors.WithMessage(err, "failed get old storage")
	}
	if oldStorage.Driver != storage.Driver {
		return errors.Errorf("driver cannot be changed")
	}
	storage.Modified = time.Now()
	storage.MountPath = utils.FixAndCleanPath(storage.MountPath)
	err = db.UpdateStorage(&storage)
	if err != nil {
		return errors.WithMessage(err, "failed update storage in database")
	}
	if storage.Disabled {
		return nil
	}
	storageDriver, err := GetStorageByMountPath(oldStorage.MountPath)
	if oldStorage.MountPath != storage.MountPath {
		// mount path renamed, need to drop the storage
		storagesMap.Delete(oldStorage.MountPath)
	}
	if err != nil {
		return errors.WithMessage(err, "failed get storage driver")
	}
	err = storageDriver.Drop(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed drop storage")
	}

	err = initStorage(ctx, storage, storageDriver)
	go callStorageHooks("update", storageDriver)
	log.Debugf("storage %+v is update", storageDriver)
	return err
}

func DeleteStorageById(ctx context.Context, id uint) error {
	storage, err := db.GetStorageById(id)
	if err != nil {
		return errors.WithMessage(err, "failed get storage")
	}
	if !storage.Disabled {
		storageDriver, err := GetStorageByMountPath(storage.MountPath)
		if err != nil {
			return errors.WithMessage(err, "failed get storage driver")
		}
		// drop the storage in the driver
		if err := storageDriver.Drop(ctx); err != nil {
			return errors.Wrapf(err, "failed drop storage")
		}
		// delete the storage in the memory
		storagesMap.Delete(storage.MountPath)
		go callStorageHooks("del", storageDriver)
	}
	// delete the storage in the database
	if err := db.DeleteStorageById(id); err != nil {
		return errors.WithMessage(err, "failed delete storage in database")
	}
	return nil
}

// MustSaveDriverStorage call from specific driver
func MustSaveDriverStorage(driver driver.Driver) {
	err := saveDriverStorage(driver)
	if err != nil {
		log.Errorf("failed save driver storage: %s", err)
	}
}

func saveDriverStorage(driver driver.Driver) error {
	storage := driver.GetStorage()
	addition := driver.GetAddition()
	str, err := utils.Json.MarshalToString(addition)
	if err != nil {
		return errors.Wrap(err, "error while marshal addition")
	}
	storage.Addition = str
	err = db.UpdateStorage(storage)
	if err != nil {
		return errors.WithMessage(err, "failed update storage in database")
	}
	return nil
}

// getStoragesByPath get storage by longest match path, contains balance storage.
// for example, there is /a/b,/a/c,/a/d/e,/a/d/e.balance
// getStoragesByPath(/a/d/e/f) => /a/d/e,/a/d/e.balance
func getStoragesByPath(path string) []driver.Driver {
	storages := make([]driver.Driver, 0)
	curSlashCount := 0
	storagesMap.Range(func(mountPath string, value driver.Driver) bool {
		mountPath = utils.GetActualMountPath(mountPath)
		// is this path
		if utils.IsSubPath(mountPath, path) {
			slashCount := strings.Count(utils.PathAddSeparatorSuffix(mountPath), "/")
			// not the longest match
			if slashCount > curSlashCount {
				storages = storages[:0]
				curSlashCount = slashCount
			}
			if slashCount == curSlashCount {
				storages = append(storages, value)
			}
		}
		return true
	})
	// make sure the order is the same for same input
	sort.Slice(storages, func(i, j int) bool {
		return storages[i].GetStorage().MountPath < storages[j].GetStorage().MountPath
	})
	return storages
}

// GetStorageVirtualFilesByPath Obtain the virtual file generated by the storage according to the path
// for example, there are: /a/b,/a/c,/a/d/e,/a/b.balance1,/av
// GetStorageVirtualFilesByPath(/a) => b,c,d
func GetStorageVirtualFilesByPath(prefix string) []model.Obj {
	files := make([]model.Obj, 0)
	storages := storagesMap.Values()
	sort.Slice(storages, func(i, j int) bool {
		if storages[i].GetStorage().Order == storages[j].GetStorage().Order {
			return storages[i].GetStorage().MountPath < storages[j].GetStorage().MountPath
		}
		return storages[i].GetStorage().Order < storages[j].GetStorage().Order
	})

	prefix = utils.FixAndCleanPath(prefix)
	set := mapset.NewSet[string]()
	for _, v := range storages {
		mountPath := utils.GetActualMountPath(v.GetStorage().MountPath)
		// Exclude prefix itself and non prefix
		if len(prefix) >= len(mountPath) || !utils.IsSubPath(prefix, mountPath) {
			continue
		}
		name := strings.SplitN(strings.TrimPrefix(mountPath[len(prefix):], "/"), "/", 2)[0]
		if set.Add(name) {
			files = append(files, &model.Object{
				Name:     name,
				Size:     0,
				Modified: v.GetStorage().Modified,
				IsFolder: true,
			})
		}
	}
	return files
}

var balanceMap generic_sync.MapOf[string, int]

// GetBalancedStorage get storage by path
func GetBalancedStorage(path string) driver.Driver {
	path = utils.FixAndCleanPath(path)
	storages := getStoragesByPath(path)
	storageNum := len(storages)
	switch storageNum {
	case 0:
		return nil
	case 1:
		return storages[0]
	default:
		virtualPath := utils.GetActualMountPath(storages[0].GetStorage().MountPath)
		i, _ := balanceMap.LoadOrStore(virtualPath, 0)
		i = (i + 1) % storageNum
		balanceMap.Store(virtualPath, i)
		return storages[i]
	}
}
