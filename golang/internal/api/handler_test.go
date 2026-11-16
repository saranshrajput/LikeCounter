package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saranshrajput/likecounter/internal/api"
	"github.com/saranshrajput/likecounter/internal/likes"
)

func TestHTTPStatusCodes(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://like_counter:like_counter@localhost:5438/like_counter"
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := likes.InitDatabase(ctx, pool, true); err != nil { // seeds post-1, user-1..3
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.New(likes.NewService(pool)))
	defer srv.Close()

	do := func(method, path, body string) int {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"first like", "POST", "/posts/post-1/like", `{"userId":"user-1"}`, 201},
		{"duplicate like", "POST", "/posts/post-1/like", `{"userId":"user-1"}`, 200},
		{"get post", "GET", "/posts/post-1", "", 200},
		{"unlike", "DELETE", "/posts/post-1/like", `{"userId":"user-1"}`, 200},
		{"unlike again", "DELETE", "/posts/post-1/like", `{"userId":"user-1"}`, 200},
		{"unknown post", "POST", "/posts/nope/like", `{"userId":"user-1"}`, 404},
		{"unknown user", "POST", "/posts/post-1/like", `{"userId":"ghost"}`, 404},
		{"get unknown post", "GET", "/posts/nope", "", 404},
		{"blank userId", "POST", "/posts/post-1/like", `{"userId":""}`, 400},
		{"bad json", "POST", "/posts/post-1/like", `not json`, 400},
	}
	for _, c := range cases {
		if got := do(c.method, c.path, c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
}
