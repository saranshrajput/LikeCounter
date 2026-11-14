const { test, before, beforeEach, after } = require('node:test');
const assert = require('node:assert/strict');
const { pool, initDatabase } = require('../src/db');
const service = require('../src/postLikeService');

before(() => initDatabase({ seed: false }));
after(() => pool.end());

beforeEach(async () => {
  await pool.query('TRUNCATE post_likes, posts, users RESTART IDENTITY CASCADE');
  await pool.query(`INSERT INTO users (id, name) VALUES ('user-1','Ada Lovelace'),('user-2','Grace Hopper')`);
  await pool.query(`INSERT INTO posts (id, title, like_count) VALUES ('post-1','Scaling counters with Postgres',0)`);
});

async function countPostLikes(postId, userId) {
  const { rows } = await pool.query(
    'SELECT COUNT(*)::int AS c FROM post_likes WHERE post_id=$1 AND user_id=$2',
    [postId, userId]
  );
  return rows[0].c;
}

const runConcurrently = (n, op) => Promise.all(Array.from({ length: n }, op));

test('increments visible like count when user likes post', async () => {
  await service.likePost('post-1', 'user-1');
  assert.equal((await service.getPost('post-1')).likeCount, 1);
});

test('does not increment twice for sequential duplicate likes', async () => {
  await service.likePost('post-1', 'user-1');
  await service.likePost('post-1', 'user-1');
  assert.equal(await countPostLikes('post-1', 'user-1'), 1);
  assert.equal((await service.getPost('post-1')).likeCount, 1);
});

test('does not decrement when user has not liked post', async () => {
  await service.unlikePost('post-1', 'user-1');
  assert.equal((await service.getPost('post-1')).likeCount, 0);
});

test('handles concurrent duplicate likes from same user', async () => {
  await runConcurrently(20, () => service.likePost('post-1', 'user-1'));
  assert.equal(await countPostLikes('post-1', 'user-1'), 1);
  assert.equal((await service.getPost('post-1')).likeCount, 1);
});

test('handles concurrent unlikes from same user', async () => {
  await service.likePost('post-1', 'user-1');
  await runConcurrently(20, () => service.unlikePost('post-1', 'user-1'));
  assert.equal(await countPostLikes('post-1', 'user-1'), 0);
  assert.equal((await service.getPost('post-1')).likeCount, 0);
});
