package op

import (
	"fmt"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/pkg/errors"
	"strconv"
	"strings"
)

func GetLabelByFileName(userId uint, fileName string) ([]model.Label, error) {
	labelIds, err := db.GetLabelIds(userId, fileName)
	if err != nil {
		return nil, errors.WithMessage(err, "failed get label_file_binding")
	}
	var labels []model.Label
	if len(labelIds) > 0 {
		if labels, err = db.GetLabelByIds(labelIds); err != nil {
			return nil, errors.WithMessage(err, "failed labels in database")
		}
	}
	return labels, nil
}

func CreateLabelFileBinDing(labelIds, fileName string, userId uint) error {
	labelMap := strings.Split(labelIds, ",")
	for _, value := range labelMap {
		labelId, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid label ID '%s': %v", value, err)
		}
		if err = db.CreateLabelFileBinDing(fileName, uint(labelId), userId); err != nil {
			return errors.WithMessage(err, "failed labels in database")
		}
	}
	return nil
}
