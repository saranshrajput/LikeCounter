CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS posts (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    like_count INTEGER NOT NULL DEFAULT 0
);

-- Safety net: the counter can never go negative, whatever the application does.
-- (Drop + add keeps this idempotent for databases created before the constraint existed.)
ALTER TABLE posts DROP CONSTRAINT IF EXISTS posts_like_count_non_negative;
ALTER TABLE posts ADD CONSTRAINT posts_like_count_non_negative CHECK (like_count >= 0);

CREATE TABLE IF NOT EXISTS post_likes (
    id BIGSERIAL PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES posts(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The real concurrency guard: at most one like per (post, user), enforced by the
-- database so it holds across any number of app servers. Also serves post_id lookups.
CREATE UNIQUE INDEX IF NOT EXISTS post_likes_post_user_uq ON post_likes(post_id, user_id);
DROP INDEX IF EXISTS post_likes_post_id_idx;
CREATE INDEX IF NOT EXISTS post_likes_user_id_idx ON post_likes(user_id);
