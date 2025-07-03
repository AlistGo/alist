package op

import (
	"context"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/pkg/errors"
)

func DeleteLabelById(ctx context.Context, id, userId uint) error {
	_, err := db.GetLabelById(id)
	if err != nil {
		return errors.WithMessage(err, "failed get label")
	}

	if db.GetLabelFileBinDingByLabelIdExists(id, userId) {
		return errors.New("label have binding relationships")
	}

	// delete the label in the database
	if err := db.DeleteLabelById(id); err != nil {
		return errors.WithMessage(err, "failed delete label in database")
	}
	return nil
}
