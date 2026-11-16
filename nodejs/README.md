# Like Counter (Node.js)

Node.js port of the Java baseline in `../java`. Same schema, same API, same (naive) behaviour.

```bash
docker compose up -d
npm install
npm test
npm start          # http://localhost:3001 (override with PORT)
```

API is identical to the Java version: `GET /posts/:postId`, `POST|DELETE /posts/:postId/like` with `{"userId":"..."}`.
Use `DATABASE_URL` to override the Postgres connection (default `postgresql://like_counter:like_counter@localhost:5438/like_counter`).
