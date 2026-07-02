// Package store wraps the sqlc-generated queries with a small, intention-revealing
// API for the rest of the service.
package store

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/viralshort/go-video/db/sqlc"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, queries: sqlc.New(pool)}, nil
}

func (s *Store) Close() { s.pool.Close() }

// SetStatus updates a video's status. Failures are logged but not propagated:
// a status write should never crash a render worker (mirrors the Python behavior).
func (s *Store) SetStatus(ctx context.Context, id uuid.UUID, status string) {
	if err := s.queries.UpdateVideoStatus(ctx, sqlc.UpdateVideoStatusParams{
		Status: status,
		ID:     id,
	}); err != nil {
		log.Printf("[%s] db status update failed -> %s: %v", id, status, err)
	}
}

// SetProgress updates a video's render progress (0-100). Failures are logged
// but not propagated, same as SetStatus.
func (s *Store) SetProgress(ctx context.Context, id uuid.UUID, pct int) {
	if err := s.queries.UpdateVideoProgress(ctx, sqlc.UpdateVideoProgressParams{
		Progress: int32(pct),
		ID:       id,
	}); err != nil {
		log.Printf("[%s] db progress update failed -> %d%%: %v", id, pct, err)
	}
}
