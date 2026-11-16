const { test, before, beforeEach, after } = require('node:test');
const assert = require('node:assert/strict');
const { pool, initDatabase } = require('../src/db');
const service = require('../src/postLikeService');

const USERS = 50;

before(() => initDatabase({ seed: false }));
after(() => pool.end());

beforeEach(async () => {
  await pool.query('TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE');
  await pool.query(
    `INSERT INTO users (id, name) SELECT 'user-' || g, 'User ' || g FROM generate_series(1, ${USERS}) g`
  );
  await pool.query(`INSERT INTO posts (id, title, like_count) VALUES ('post-1','Scaling counters with Postgres',0)`);
});

const likeCount = async (postId) => (await service.getPost(postId)).likeCount;
const q = async (sql, params) => (await pool.query(sql, params)).rows[0].c;
const countPostLikes = (postId, userId) =>
  q('SELECT COUNT(*)::int AS c FROM post_likes WHERE post_id=$1 AND user_id=$2', [postId, userId]);
const totalLikeRows = (postId) => q('SELECT COUNT(*)::int AS c FROM post_likes WHERE post_id=$1', [postId]);

// Fires all tasks at once; rejects if any task rejects. With a pool of 10, up to 10 run truly in parallel on the DB.
const runConcurrently = (n, task) => Promise.all(Array.from({ length: n }, (_, i) => task(i)));

test('increments visible like count when user likes post', async () => {
  const r = await service.likePost('post-1', 'user-1');
  assert.equal(r.created, true);
  assert.equal(await likeCount('post-1'), 1);
});

test('does not increment twice for sequential duplicate likes', async () => {
  const a = await service.likePost('post-1', 'user-1');
  const b = await service.likePost('post-1', 'user-1');
  assert.equal(a.created, true);
  assert.equal(b.created, false);
  assert.equal(await countPostLikes('post-1', 'user-1'), 1);
  assert.equal(await likeCount('post-1'), 1);
});

test('does not decrement when user has not liked post', async () => {
  const r = await service.unlikePost('post-1', 'user-1');
  assert.equal(r.deleted, false);
  assert.equal(await likeCount('post-1'), 0);
});

test('unlike removes like and decrements once', async () => {
  await service.likePost('post-1', 'user-1');
  assert.equal((await service.unlikePost('post-1', 'user-1')).deleted, true);
  assert.equal((await service.unlikePost('post-1', 'user-1')).deleted, false);
  assert.equal(await likeCount('post-1'), 0);
});

test('handles concurrent duplicate likes from same user', async () => {
  const results = await runConcurrently(20, () => service.likePost('post-1', 'user-1'));
  assert.equal(results.filter((r) => r.created).length, 1);
  assert.equal(await countPostLikes('post-1', 'user-1'), 1);
  assert.equal(await likeCount('post-1'), 1);
});

test('handles concurrent unlikes from same user', async () => {
  await service.likePost('post-1', 'user-1');
  const results = await runConcurrently(20, () => service.unlikePost('post-1', 'user-1'));
  assert.equal(results.filter((r) => r.deleted).length, 1);
  assert.equal(await countPostLikes('post-1', 'user-1'), 0);
  assert.equal(await likeCount('post-1'), 0);
});

test('handles concurrent unlikes when user never liked', async () => {
  await runConcurrently(20, () => service.unlikePost('post-1', 'user-1'));
  assert.equal(await likeCount('post-1'), 0);
});

test('handles concurrent likes from many distinct users', async () => {
  await runConcurrently(USERS, (i) => service.likePost('post-1', `user-${i + 1}`));
  assert.equal(await likeCount('post-1'), USERS);
  assert.equal(await totalLikeRows('post-1'), USERS);
});

test('keeps counter equal to rows under mixed concurrent likes and unlikes', async () => {
  for (let u = 1; u <= 10; u++) await service.likePost('post-1', `user-${u}`);

  await runConcurrently(200, (i) => {
    const user = `user-${(i % USERS) + 1}`;
    return i % 2 === 0 ? service.likePost('post-1', user) : service.unlikePost('post-1', user);
  });

  const rows = await totalLikeRows('post-1');
  const count = await likeCount('post-1');
  assert.equal(count, rows);
  assert.ok(count >= 0);
});

test('rejects unknown post or user with NotFoundError', async () => {
  await assert.rejects(service.likePost('nope', 'user-1'), { name: 'NotFoundError' });
  await assert.rejects(service.likePost('post-1', 'ghost'), { name: 'NotFoundError' });
  await assert.rejects(service.unlikePost('nope', 'user-1'), { name: 'NotFoundError' });
});

test('database rejects negative counter and duplicate like rows', async () => {
  await assert.rejects(pool.query(`UPDATE posts SET like_count = -1 WHERE id = 'post-1'`), { code: '23514' });
  await pool.query(`INSERT INTO post_likes (post_id, user_id) VALUES ('post-1','user-1')`);
  await assert.rejects(
    pool.query(`INSERT INTO post_likes (post_id, user_id) VALUES ('post-1','user-1')`),
    { code: '23505' }
  );
});
