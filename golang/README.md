# Like Counter (Go)

Go port of the Java and Node.js versions in `../java` and `../nodejs`. Same schema, same API, same guarantees.

- Go 1.22+ (uses `net/http` `ServeMux` method patterns), `pgx/v5` + `pgxpool`, standard `testing`.
- Layout: `cmd/server` (main), `internal/api` (HTTP), `internal/likes` (service + SQL + embedded schema).

```bash
docker compose up -d
go test ./...
go run ./cmd/server        # http://localhost:3002 (override with PORT)
```

`DATABASE_URL` overrides the connection (default `postgres://like_counter:like_counter@localhost:5438/like_counter`).

API is identical to the other versions: `GET /posts/{postId}`, `POST|DELETE /posts/{postId}/like` with `{"userId":"..."}`.

Note: `go test ./...` runs packages in parallel by default and both test packages truncate shared tables, so use `go test -p 1 ./...` against one database.
