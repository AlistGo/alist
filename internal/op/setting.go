package op

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/singleflight"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
)

var settingCache = cache.NewMemCache(cache.WithShards[*model.SettingItem](4))
var settingG singleflight.Group[*model.SettingItem]
var settingCacheF = func(item *model.SettingItem) {
	settingCache.Set(item.Key, item, cache.WithEx[*model.SettingItem](time.Hour))
}

var settingGroupCache = cache.NewMemCache(cache.WithShards[[]model.SettingItem](4))
var settingGroupG singleflight.Group[[]model.SettingItem]
var settingGroupCacheF = func(key string, item []model.SettingItem) {
	settingGroupCache.Set(key, item, cache.WithEx[[]model.SettingItem](time.Hour))
}

var settingChangingCallbacks = make([]func(), 0)

func RegisterSettingChangingCallback(f func()) {
	settingChangingCallbacks = append(settingChangingCallbacks, f)
}

func SettingCacheUpdate() {
	settingCache.Clear()
	settingGroupCache.Clear()
	for _, cb := range settingChangingCallbacks {
		cb()
	}
}

func GetPublicSettingsMap() map[string]string {
	items, _ := GetPublicSettingItems()
	pSettings := make(map[string]string)
	for _, item := range items {
		pSettings[item.Key] = item.Value
	}
	return pSettings
}

func GetSettingsMap() map[string]string {
	items, _ := GetSettingItems()
	settings := make(map[string]string)
	for _, item := range items {
		settings[item.Key] = item.Value
	}
	return settings
}

func GetSettingItems() ([]model.SettingItem, error) {
	if items, ok := settingGroupCache.Get("ALL_SETTING_ITEMS"); ok {
		return items, nil
	}
	items, err, _ := settingGroupG.Do("ALL_SETTING_ITEMS", func() ([]model.SettingItem, error) {
		_items, err := db.GetSettingItems()
		if err != nil {
			return nil, err
		}
		settingGroupCacheF("ALL_SETTING_ITEMS", _items)
		return _items, nil
	})
	return items, err
}

func GetPublicSettingItems() ([]model.SettingItem, error) {
	if items, ok := settingGroupCache.Get("ALL_PUBLIC_SETTING_ITEMS"); ok {
		return items, nil
	}
	items, err, _ := settingGroupG.Do("ALL_PUBLIC_SETTING_ITEMS", func() ([]model.SettingItem, error) {
		_items, err := db.GetPublicSettingItems()
		if err != nil {
			return nil, err
		}
		settingGroupCacheF("ALL_PUBLIC_SETTING_ITEMS", _items)
		return _items, nil
	})
	return items, err
}

func GetSettingItemByKey(key string) (*model.SettingItem, error) {
	if item, ok := settingCache.Get(key); ok {
		return item, nil
	}

	item, err, _ := settingG.Do(key, func() (*model.SettingItem, error) {
		_item, err := db.GetSettingItemByKey(key)
		if err != nil {
			return nil, err
		}
		settingCacheF(_item)
		return _item, nil
	})
	return item, err
}

func GetSettingItemInKeys(keys []string) ([]model.SettingItem, error) {
	var items []model.SettingItem
	for _, key := range keys {
		item, err := GetSettingItemByKey(key)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func GetSettingItemsByGroup(group int) ([]model.SettingItem, error) {
	key := strconv.Itoa(group)
	if items, ok := settingGroupCache.Get(key); ok {
		return items, nil
	}
	items, err, _ := settingGroupG.Do(key, func() ([]model.SettingItem, error) {
		_items, err := db.GetSettingItemsByGroup(group)
		if err != nil {
			return nil, err
		}
		settingGroupCacheF(key, _items)
		return _items, nil
	})
	return items, err
}

func GetSettingItemsInGroups(groups []int) ([]model.SettingItem, error) {
	sort.Ints(groups)
	key := strings.Join(utils.MustSliceConvert(groups, func(i int) string {
		return strconv.Itoa(i)
	}), ",")

	if items, ok := settingGroupCache.Get(key); ok {
		return items, nil
	}
	items, err, _ := settingGroupG.Do(key, func() ([]model.SettingItem, error) {
		_items, err := db.GetSettingItemsInGroups(groups)
		if err != nil {
			return nil, err
		}
		settingGroupCacheF(key, _items)
		return _items, nil
	})
	return items, err
}

func SaveSettingItems(items []model.SettingItem) error {
	noHookItems := make([]model.SettingItem, 0)
	errs := make([]error, 0)
	for i := range items {
		if ok, err := HandleSettingItemHook(&items[i]); ok {
			if err != nil {
				errs = append(errs, err)
			} else {
				err = db.SaveSettingItem(&items[i])
				if err != nil {
					errs = append(errs, err)
				}
			}
		} else {
			noHookItems = append(noHookItems, items[i])
		}
	}
	if len(noHookItems) > 0 {
		err := db.SaveSettingItems(noHookItems)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) < len(items)-len(noHookItems)+1 {
		SettingCacheUpdate()
	}
	return utils.MergeErrors(errs...)
}

func SaveSettingItem(item *model.SettingItem) (err error) {
	// hook
	if _, err := HandleSettingItemHook(item); err != nil {
		return err
	}
	// update
	if err = db.SaveSettingItem(item); err != nil {
		return err
	}
	SettingCacheUpdate()
	return nil
}

func DeleteSettingItemByKey(key string) error {
	old, err := GetSettingItemByKey(key)
	if err != nil {
		return errors.WithMessage(err, "failed to get settingItem")
	}
	if !old.IsDeprecated() {
		return errors.Errorf("setting [%s] is not deprecated", key)
	}
	SettingCacheUpdate()
	return db.DeleteSettingItemByKey(key)
}
