// Package likes holds the like/unlike business logic and the SQL behind it.
package likes

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed sql/schema.sql
var schemaSQL string

//go:embed sql/data.sql
var seedSQL string

var (
	ErrPostNotFound = errors.New("post not found")
	ErrUserNotFound = errors.New("user not found")
)

type Post struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	LikeCount int    `json:"likeCount"`
}

type LikeResult struct {
	PostID  string `json:"postId"`
	UserID  string `json:"userId"`
	Created bool   `json:"created"`
	Deleted bool   `json:"deleted"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// InitDatabase applies schema.sql and, when seed is true, data.sql (which truncates!).
// Exec with no arguments uses the simple protocol, so multi-statement scripts are fine.
func InitDatabase(ctx context.Context, pool *pgxpool.Pool, seed bool) error {
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	if seed {
		if _, err := pool.Exec(ctx, seedSQL); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	return nil
}

// GetPost is a primary-key lookup on the denormalized counter: O(1) regardless of like volume.
// Returns (nil, nil) when the post does not exist.
func (s *Service) GetPost(ctx context.Context, postID string) (*Post, error) {
	var p Post
	err := s.pool.QueryRow(ctx,
		`SELECT id, title, like_count FROM posts WHERE id = $1`, postID,
	).Scan(&p.ID, &p.Title, &p.LikeCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// LikePost is idempotent. The unique index on (post_id, user_id) decides who wins a race:
// only the request whose INSERT actually created a row bumps the counter, and both
// statements commit together.
func (s *Service) LikePost(ctx context.Context, postID, userID string) (LikeResult, error) {
	res := LikeResult{PostID: postID, UserID: userID}

	// BeginFunc commits if fn returns nil, otherwise rolls back (Go's "defer tx.Rollback()" pattern, built in).
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireExists(ctx, tx, "posts", postID, ErrPostNotFound); err != nil {
			return err
		}
		if err := requireExists(ctx, tx, "users", userID, ErrUserNotFound); err != nil {
			return err
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO post_likes (post_id, user_id) VALUES ($1, $2)
			ON CONFLICT (post_id, user_id) DO NOTHING`, postID, userID)
		if err != nil {
			return err
		}

		if tag.RowsAffected() == 1 { // only the race winner increments
			if _, err := tx.Exec(ctx,
				`UPDATE posts SET like_count = like_count + 1 WHERE id = $1`, postID); err != nil {
				return err
			}
			res.Created = true
		}
		return nil
	})
	return res, err
}

// UnlikePost is idempotent: only a request that really deleted the row decrements.
func (s *Service) UnlikePost(ctx context.Context, postID, userID string) (LikeResult, error) {
	res := LikeResult{PostID: postID, UserID: userID}

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireExists(ctx, tx, "posts", postID, ErrPostNotFound); err != nil {
			return err
		}

		tag, err := tx.Exec(ctx,
			`DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2`, postID, userID)
		if err != nil {
			return err
		}

		if tag.RowsAffected() == 1 {
			if _, err := tx.Exec(ctx,
				`UPDATE posts SET like_count = like_count - 1 WHERE id = $1`, postID); err != nil {
				return err
			}
			res.Deleted = true
		}
		return nil
	})
	return res, err
}

// table is only ever a hard-coded literal from this file, never user input.
func requireExists(ctx context.Context, tx pgx.Tx, table, id string, notFound error) error {
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return notFound
	}
	return nil
}
