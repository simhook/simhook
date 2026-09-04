package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

func notFoundIfNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
