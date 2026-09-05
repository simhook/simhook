// Package store is the only place SQL lives. Each file covers one aggregate.
// Methods take a context and return typed rows; callers never see pgx.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup matches nothing.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a uniqueness rule rejects a write.
var ErrConflict = errors.New("conflict")

// Querier is satisfied by both the pool and a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store runs queries against a pool, or against a transaction when built
// with WithTx.
type Store struct {
	q    Querier
	pool *pgxpool.Pool
}

// New returns a Store bound to the pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{q: pool, pool: pool}
}

// WithTx returns a Store whose queries run inside tx.
func (s *Store) WithTx(tx pgx.Tx) *Store {
	return &Store{q: tx, pool: s.pool}
}

// Tx runs fn inside a transaction, committing on nil error.
func (s *Store) Tx(ctx context.Context, fn func(tx pgx.Tx, st *Store) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx, s.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func one[T any](rows pgx.Rows, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	v, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, ErrNotFound
		}
		return zero, err
	}
	return v, nil
}

func many[T any](rows pgx.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

// isUniqueViolation reports whether err is a unique constraint failure.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func wrapWrite(err error) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
