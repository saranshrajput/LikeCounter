# Solution Notes (Go / net/http + pgx)

## Assumptions

- Users and posts already exist (created elsewhere); like/unlike on an unknown post or user returns **404**.
- `userId` comes from the request body for this exercise. In production it would come from an authenticated session/token, never from the client.
- Like and unlike are idempotent: repeating a request is safe and returns `created:false` / `deleted:false`.
- "Reasonably fresh" means: a user sees their own like immediately (read-your-writes), and other viewers may be up to ~1-2 s behind **only in the scaled design below**. The current implementation is exact.
- Postgres is the single source of truth; several stateless app servers share it.

## Changes

| Problem in the original | Fix |
|---|---|
| Check-then-insert race (`SELECT COUNT` then `INSERT`) let two requests both insert | Unique index on `post_likes(post_id, user_id)` + `INSERT ... ON CONFLICT DO NOTHING`. The database picks one winner, regardless of how many app servers race. |
| Unlike always did `like_count - 1` | `DELETE` returns the number of rows removed; decrement **only if 1 row was deleted**. |
| Counter could go negative | `CHECK (like_count >= 0)` on `posts` as a last line of defence. |
| Row insert and counter update were separate commits | Both run in one transaction (`@Transactional` in Java, `BEGIN/COMMIT` helper in Node, `pgx.BeginFunc` in Go). They commit or roll back together, so `like_count == COUNT(post_likes)` always holds. |
| Counter update was not atomic by construction | `SET like_count = like_count ± 1` is done inside SQL (no read-modify-write in app code). |
| Unknown post/user caused a 500 (FK violation) | Existence checks return 404. |
| Redundant index `post_likes(post_id)` | Dropped; the new unique index covers it. |

Reads are unchanged: `GET /posts/:id` is a primary-key lookup of a denormalized counter, O(1) no matter how many likes exist.

## Tests

Run against a real Postgres (not mocks), because the guarantees come from the database.

- sequential duplicate like, unlike without a like, like/unlike round trip
- 20 concurrent duplicate likes by one user -> exactly one `created:true`, one row, count 1
- 20 concurrent unlikes -> exactly one `deleted:true`, count 0; and 20 concurrent unlikes with no like -> count stays 0
- 50 concurrent likes from 50 distinct users -> count 50 and 50 rows
- 200 mixed concurrent likes/unlikes (with duplicates) -> invariant `like_count == COUNT(post_likes)` and `>= 0`
- 404 for unknown post/user
- the database itself rejects a negative counter and a duplicate (post, user) row

The concurrency tests use a real thread pool released by a start latch (Java), simultaneous promises on a 10-connection pool (Node), or goroutines released by a closed "start" channel (Go), so requests genuinely overlap. The DB constraints are the thing under test, not the test timing.

## Exact vs eventual consistency

**Current design: exact.** The membership row and the counter change in the same transaction, so any reader sees a count that matches the rows. The price is that every like of a post updates the same `posts` row.

**Scaled design: eventual (bounded staleness).** The `post_likes` rows stay exact and are the source of truth (uniqueness is still enforced by the unique index). The *displayed* count becomes an approximation that converges within seconds. Likes are never lost and never double counted; only the number shown on the page lags briefly. This is acceptable for like counts, not for something like account balances.

## Tradeoffs

- **Denormalized counter vs `COUNT(*)`**: O(1) reads vs. write contention on one row. `COUNT(*)` on a post with millions of likes is far too slow for the hot read path.
- **Unique index vs locks/SERIALIZABLE**: the index is cheaper, simpler and correct across servers. Advisory locks or SERIALIZABLE retries would add load and complexity for no extra guarantee here.
- **Existence checks** cost two extra primary-key lookups per like; they buy clean 404s. Under heavy load they could be dropped in favour of mapping FK violations to 404.
- **Exact vs fast**: exactness is simplest and right for the current scale; relaxing it is only worth the complexity once a hot row becomes the bottleneck.

## What breaks for a viral post

- Every like/unlike takes a row lock on that post's `posts` row, so writes to it **serialize**. Throughput is capped at roughly the inverse of the transaction time (order of hundreds to low thousands of likes/s), regardless of how many app servers you add.
- Requests queue behind the lock, holding DB connections; the pool (10 per app server) saturates, latency spikes, and unrelated requests (including `GET`) start timing out.
- Hot-row updates create dead tuples quickly, causing table bloat and vacuum pressure.
- `post_likes` becomes a very large table; one index covers (post_id, user_id) lookups, but it grows without bound.
- Retries from clients/load balancers amplify the pile-up.

## Scaling Design

In order of effort / payoff:

1. **Cache the read path**: `GET` served from Redis or a CDN with a 1-2 s TTL. Removes read load from Postgres entirely; this is where the "reasonably fresh" tradeoff is made.
2. **Sharded counters**: replace the single counter with `post_like_counters(post_id, shard, n)` (e.g. 16 shards). A like increments a random shard, so concurrent likes hit different rows. Read = `SUM(n)` (or cached). Keeps the exact transactional guarantee while spreading lock contention ~16x.
3. **Write-behind counting**: write the membership row (guarded by the unique index), `INCR` a Redis counter, and flush deltas to Postgres in batches (e.g. every second) via a queue/worker. Counter updates become one write per post per second instead of one per like. Eventual, bounded staleness.
4. **Read replicas** for reads that tolerate lag; keep writes on the primary.
5. **Partition `post_likes`** by hash of `post_id` (or shard across databases) once a single table or node is too big.
6. **Reconciliation job**: periodically compare counters with `COUNT(*)` from `post_likes` and repair drift. This is the safety net that makes eventual counting trustworthy.
7. **Protect the system**: rate-limit per user, idempotency keys, bounded DB timeouts / circuit breakers so a hot post cannot starve the pool.

## Future Improvements

- Remove the `TRUNCATE` in `data.sql` from startup. It wipes all data every time any server boots, which is unsafe with several app servers. Use proper migrations (Flyway / node-pg-migrate) and a separate seed for dev only.
- The schema change is idempotent for fresh and existing databases, but adding the unique index to a database that already contains duplicate rows would fail; a migration would first de-duplicate and recompute `like_count` from `post_likes`.
- Authenticate users; derive `userId` from the token.
- Metrics (lock wait time, pool usage) and load tests with a single hot post to find the real ceiling before choosing between sharding and write-behind.
