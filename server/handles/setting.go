package handles

import (
	"strconv"
	"strings"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/alist-org/alist/v3/internal/sign"
	"github.com/alist-org/alist/v3/pkg/utils/random"
	"github.com/alist-org/alist/v3/server/common"
	"github.com/alist-org/alist/v3/server/static"
	"github.com/gin-gonic/gin"
)

func getRoleOptions() string {
	roles, _, err := op.GetRoles(1, model.MaxInt)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		if r.Name == "admin" || r.Name == "guest" {
			continue
		}
		names = append(names, r.Name)
	}
	return strings.Join(names, ",")
}

func ResetToken(c *gin.Context) {
	token := random.Token()
	item := model.SettingItem{Key: "token", Value: token, Type: conf.TypeString, Group: model.SINGLE, Flag: model.PRIVATE}
	if err := op.SaveSettingItem(&item); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	sign.Instance()
	common.SuccessResp(c, token)
}

func GetSetting(c *gin.Context) {
	key := c.Query("key")
	keys := c.Query("keys")
	if key != "" {
		item, err := op.GetSettingItemByKey(key)
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		if item.Key == conf.DefaultRole {
			copy := *item
			copy.Options = getRoleOptions()
			if id, err := strconv.Atoi(copy.Value); err == nil {
				if r, err := op.GetRole(uint(id)); err == nil {
					copy.Value = r.Name
				}
			}
			common.SuccessResp(c, copy)
			return
		}
		common.SuccessResp(c, item)
	} else {
		items, err := op.GetSettingItemInKeys(strings.Split(keys, ","))
		if err != nil {
			common.ErrorResp(c, err, 400)
			return
		}
		for i := range items {
			if items[i].Key == conf.DefaultRole {
				if id, err := strconv.Atoi(items[i].Value); err == nil {
					if r, err := op.GetRole(uint(id)); err == nil {
						items[i].Value = r.Name
					}
				}
				items[i].Options = getRoleOptions()
				break
			}
		}
		common.SuccessResp(c, items)
	}
}

func SaveSettings(c *gin.Context) {
	var req []model.SettingItem
	if err := c.ShouldBind(&req); err != nil {
		common.ErrorResp(c, err, 400)
		return
	}

	for i := range req {
		if req[i].Key == conf.DefaultRole {
			role, err := op.GetRoleByName(req[i].Value)
			if err != nil {
				common.ErrorResp(c, err, 400)
				return
			}
			if role.Name == "admin" || role.Name == "guest" {
				common.ErrorStrResp(c, "cannot set admin or guest as default role", 400)
				return
			}
			req[i].Value = strconv.Itoa(int(role.ID))
		}
	}

	if err := op.SaveSettingItems(req); err != nil {
		common.ErrorResp(c, err, 500)
	} else {
		common.SuccessResp(c)
		static.UpdateIndex()
	}
}

func ListSettings(c *gin.Context) {
	groupStr := c.Query("group")
	groupsStr := c.Query("groups")
	var settings []model.SettingItem
	var err error
	if groupsStr == "" && groupStr == "" {
		settings, err = op.GetSettingItems()
	} else {
		var groupStrings []string
		if groupsStr != "" {
			groupStrings = strings.Split(groupsStr, ",")
		} else {
			groupStrings = append(groupStrings, groupStr)
		}
		var groups []int
		for _, str := range groupStrings {
			group, err := strconv.Atoi(str)
			if err != nil {
				common.ErrorResp(c, err, 400)
				return
			}
			groups = append(groups, group)
		}
		settings, err = op.GetSettingItemsInGroups(groups)
	}
	if err != nil {
		common.ErrorResp(c, err, 400)
		return
	}
	for i := range settings {
		if settings[i].Key == conf.DefaultRole {
			if id, err := strconv.Atoi(settings[i].Value); err == nil {
				if r, err := op.GetRole(uint(id)); err == nil {
					settings[i].Value = r.Name
				}
			}
			settings[i].Options = getRoleOptions()
			break
		}
	}
	common.SuccessResp(c, settings)
}

func DeleteSetting(c *gin.Context) {
	key := c.Query("key")
	if err := op.DeleteSettingItemByKey(key); err != nil {
		common.ErrorResp(c, err, 500)
		return
	}
	common.SuccessResp(c)
}

func PublicSettings(c *gin.Context) {
	common.SuccessResp(c, op.GetPublicSettingsMap())
}
