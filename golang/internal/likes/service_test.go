package likes_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saranshrajput/likecounter/internal/likes"
)

const users = 50

var (
	pool *pgxpool.Pool
	svc  *likes.Service
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://like_counter:like_counter@localhost:5438/like_counter"
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		panic(err)
	}
	cfg.MaxConns = 10
	pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}
	if err := likes.InitDatabase(ctx, pool, false); err != nil {
		panic(err)
	}
	svc = likes.NewService(pool)
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	stmts := []string{
		`TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE`,
		fmt.Sprintf(`INSERT INTO users (id, name) SELECT 'user-' || g, 'User ' || g FROM generate_series(1, %d) g`, users),
		`INSERT INTO posts (id, title, like_count) VALUES ('post-1', 'Scaling counters with Postgres', 0)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
}

func likeCount(t *testing.T) int {
	t.Helper()
	p, err := svc.GetPost(context.Background(), "post-1")
	if err != nil || p == nil {
		t.Fatalf("GetPost: %v %v", p, err)
	}
	return p.LikeCount
}

func rows(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM post_likes WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// runConcurrently starts n goroutines, parks them on a closed-channel "start gun",
// then releases them together so the calls genuinely overlap. Errors fail the test.
func runConcurrently(t *testing.T, n int, task func(i int) (likes.LikeResult, error)) []likes.LikeResult {
	t.Helper()
	results := make([]likes.LikeResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i], errs[i] = task(i)
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("task %d: %v", i, err)
		}
	}
	return results
}

func count(rs []likes.LikeResult, pred func(likes.LikeResult) bool) int {
	n := 0
	for _, r := range rs {
		if pred(r) {
			n++
		}
	}
	return n
}

func TestIncrementsVisibleLikeCount(t *testing.T) {
	reset(t)
	r, err := svc.LikePost(context.Background(), "post-1", "user-1")
	if err != nil || !r.Created {
		t.Fatalf("got %+v, %v", r, err)
	}
	if got := likeCount(t); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
}

func TestSequentialDuplicateLikes(t *testing.T) {
	reset(t)
	ctx := context.Background()
	a, _ := svc.LikePost(ctx, "post-1", "user-1")
	b, _ := svc.LikePost(ctx, "post-1", "user-1")
	if !a.Created || b.Created {
		t.Fatalf("created flags: %v %v", a.Created, b.Created)
	}
	if rows(t, `post_id='post-1' AND user_id='user-1'`) != 1 || likeCount(t) != 1 {
		t.Fatal("duplicate like changed state")
	}
}

func TestUnlikeWithoutLikeDoesNotDecrement(t *testing.T) {
	reset(t)
	r, err := svc.UnlikePost(context.Background(), "post-1", "user-1")
	if err != nil || r.Deleted {
		t.Fatalf("got %+v, %v", r, err)
	}
	if got := likeCount(t); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
}

func TestUnlikeRemovesLikeAndDecrementsOnce(t *testing.T) {
	reset(t)
	ctx := context.Background()
	_, _ = svc.LikePost(ctx, "post-1", "user-1")
	a, _ := svc.UnlikePost(ctx, "post-1", "user-1")
	b, _ := svc.UnlikePost(ctx, "post-1", "user-1")
	if !a.Deleted || b.Deleted || likeCount(t) != 0 {
		t.Fatalf("a=%+v b=%+v count=%d", a, b, likeCount(t))
	}
}

func TestConcurrentDuplicateLikesSameUser(t *testing.T) {
	reset(t)
	rs := runConcurrently(t, 20, func(int) (likes.LikeResult, error) {
		return svc.LikePost(context.Background(), "post-1", "user-1")
	})
	if n := count(rs, func(r likes.LikeResult) bool { return r.Created }); n != 1 {
		t.Fatalf("created true %d times, want 1", n)
	}
	if rows(t, `post_id='post-1' AND user_id='user-1'`) != 1 || likeCount(t) != 1 {
		t.Fatalf("rows/count wrong: count=%d", likeCount(t))
	}
}

func TestConcurrentUnlikesSameUser(t *testing.T) {
	reset(t)
	_, _ = svc.LikePost(context.Background(), "post-1", "user-1")
	rs := runConcurrently(t, 20, func(int) (likes.LikeResult, error) {
		return svc.UnlikePost(context.Background(), "post-1", "user-1")
	})
	if n := count(rs, func(r likes.LikeResult) bool { return r.Deleted }); n != 1 {
		t.Fatalf("deleted true %d times, want 1", n)
	}
	if rows(t, `post_id='post-1'`) != 0 || likeCount(t) != 0 {
		t.Fatalf("count = %d", likeCount(t))
	}
}

func TestConcurrentUnlikesWhenNeverLiked(t *testing.T) {
	reset(t)
	runConcurrently(t, 20, func(int) (likes.LikeResult, error) {
		return svc.UnlikePost(context.Background(), "post-1", "user-1")
	})
	if got := likeCount(t); got != 0 {
		t.Fatalf("count = %d, want 0", got)
	}
}

func TestConcurrentLikesFromManyUsers(t *testing.T) {
	reset(t)
	runConcurrently(t, users, func(i int) (likes.LikeResult, error) {
		return svc.LikePost(context.Background(), "post-1", fmt.Sprintf("user-%d", i+1))
	})
	if likeCount(t) != users || rows(t, `post_id='post-1'`) != users {
		t.Fatalf("count=%d rows=%d, want %d", likeCount(t), rows(t, `post_id='post-1'`), users)
	}
}

func TestMixedConcurrentLikesAndUnlikesKeepInvariant(t *testing.T) {
	reset(t)
	ctx := context.Background()
	for u := 1; u <= 10; u++ {
		_, _ = svc.LikePost(ctx, "post-1", fmt.Sprintf("user-%d", u))
	}
	runConcurrently(t, 200, func(i int) (likes.LikeResult, error) {
		user := fmt.Sprintf("user-%d", i%users+1)
		if i%2 == 0 {
			return svc.LikePost(ctx, "post-1", user)
		}
		return svc.UnlikePost(ctx, "post-1", user)
	})
	c, r := likeCount(t), rows(t, `post_id='post-1'`)
	if c != r || c < 0 {
		t.Fatalf("invariant broken: like_count=%d rows=%d", c, r)
	}
}

func TestNotFound(t *testing.T) {
	reset(t)
	ctx := context.Background()
	if _, err := svc.LikePost(ctx, "nope", "user-1"); !errors.Is(err, likes.ErrPostNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.LikePost(ctx, "post-1", "ghost"); !errors.Is(err, likes.ErrUserNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.UnlikePost(ctx, "nope", "user-1"); !errors.Is(err, likes.ErrPostNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestDatabaseRejectsNegativeCounterAndDuplicateRows(t *testing.T) {
	reset(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `UPDATE posts SET like_count = -1 WHERE id = 'post-1'`)
	assertPgCode(t, err, "23514") // check_violation

	if _, err := pool.Exec(ctx, `INSERT INTO post_likes (post_id, user_id) VALUES ('post-1','user-1')`); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO post_likes (post_id, user_id) VALUES ('post-1','user-1')`)
	assertPgCode(t, err, "23505") // unique_violation
}

func assertPgCode(t *testing.T, err error, code string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("want pg error %s, got %v", code, err)
	}
}
