TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE;

INSERT INTO users (id, name) VALUES
    ('user-1', 'Ada Lovelace'),
    ('user-2', 'Grace Hopper'),
    ('user-3', 'Katherine Johnson');

INSERT INTO posts (id, title, like_count) VALUES
    ('post-1', 'Scaling counters with Postgres', 0),
    ('post-2', 'Concurrency bugs are product bugs', 0);
