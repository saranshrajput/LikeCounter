# Like Counter

This repo contains a small Postgres-backed service for post like counts.

The current implementation works for simple local usage, but it needs to be made correct and scalable for real traffic.

## Requirements

- A user can like a post at most once.
- Unliking should not corrupt the count, even if the user has not liked the post.
- `posts.like_count` must never go negative.
- `GET /posts/:postId` should be fast.
- The implementation should be safe when duplicate or concurrent requests hit multiple app servers.
- During high traffic, the displayed count should be reasonably fresh. State your assumptions.

## Setup

```bash
docker compose up -d
mvn test
mvn spring-boot:run
```

## API

```bash
curl http://localhost:3000/posts/post-1

curl -X POST http://localhost:3000/posts/post-1/like \
  -H 'content-type: application/json' \
  -d '{"userId":"user-1"}'

curl -X DELETE http://localhost:3000/posts/post-1/like \
  -H 'content-type: application/json' \
  -d '{"userId":"user-1"}'
```

## Deliverables

1. Code changes that make the current Postgres implementation correct.
2. Tests proving duplicate and concurrent like/unlike requests do not corrupt counts.
3. A short note in `SOLUTION.md` covering:
   - assumptions,
   - tradeoffs,
   - exact vs eventual consistency,
   - what breaks for a viral post,
   - how you would scale the design further.
