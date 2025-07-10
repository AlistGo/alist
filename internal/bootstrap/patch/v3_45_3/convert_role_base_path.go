package v3_45_3

import (
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
)

// ConvertRoleBasePath migrates old base_path to base_paths
func ConvertRoleBasePath() {
	if err := db.AutoMigrate(new(model.Role)); err != nil {
		utils.Log.Errorf("[convert role base path] failed to auto migrate: %v", err)
		return
	}

	type oldRole struct {
		ID        uint
		BasePath  string
		BasePaths model.Paths
	}

	var roles []oldRole
	if err := db.GetDb().Table("roles").Find(&roles).Error; err != nil {
		utils.Log.Errorf("[convert role base path] failed to query roles: %v", err)
		return
	}

	for _, r := range roles {
		if len(r.BasePaths) > 0 || r.BasePath == "" {
			continue
		}
		paths := model.Paths{utils.FixAndCleanPath(r.BasePath)}
		if err := db.GetDb().Table("roles").Where("id = ?", r.ID).Update("base_paths", paths).Error; err != nil {
			utils.Log.Errorf("[convert role base path] failed to update role %d: %v", r.ID, err)
		}
	}
}
