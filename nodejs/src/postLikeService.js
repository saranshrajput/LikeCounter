const { pool } = require('./db');

class NotFoundError extends Error {
  constructor(message) {
    super(message);
    this.name = 'NotFoundError';
    this.status = 404;
  }
}

// Runs fn(client) inside BEGIN/COMMIT on one dedicated connection; ROLLBACK on error.
// (The Java version gets this from @Transactional.)
async function withTransaction(fn) {
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const result = await fn(client);
    await client.query('COMMIT');
    return result;
  } catch (err) {
    await client.query('ROLLBACK').catch(() => {});
    throw err;
  } finally {
    client.release();
  }
}

// `table` is only ever a hard-coded literal from this file, never user input.
async function requireExists(client, table, id, message) {
  const { rows } = await client.query(`SELECT EXISTS (SELECT 1 FROM ${table} WHERE id = $1) AS e`, [id]);
  if (!rows[0].e) throw new NotFoundError(message);
}

async function getPost(postId) {
  const { rows } = await pool.query(
    'SELECT id, title, like_count FROM posts WHERE id = $1',
    [postId]
  );
  if (rows.length === 0) return null;
  const r = rows[0];
  return { id: r.id, title: r.title, likeCount: r.like_count };
}

// Idempotent like. The unique index on (post_id, user_id) decides who wins a race:
// only the request whose INSERT actually created a row bumps the counter, and both
// statements commit together.
function likePost(postId, userId) {
  return withTransaction(async (client) => {
    await requireExists(client, 'posts', postId, 'Post not found');
    await requireExists(client, 'users', userId, 'User not found');

    const ins = await client.query(
      `INSERT INTO post_likes (post_id, user_id) VALUES ($1, $2)
       ON CONFLICT (post_id, user_id) DO NOTHING`,
      [postId, userId]
    );
    if (ins.rowCount === 1) {
      await client.query('UPDATE posts SET like_count = like_count + 1 WHERE id = $1', [postId]);
    }
    return { postId, userId, created: ins.rowCount === 1, deleted: false };
  });
}

// Idempotent unlike: only a request that really deleted the row decrements.
function unlikePost(postId, userId) {
  return withTransaction(async (client) => {
    await requireExists(client, 'posts', postId, 'Post not found');

    const del = await client.query(
      'DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2',
      [postId, userId]
    );
    if (del.rowCount === 1) {
      await client.query('UPDATE posts SET like_count = like_count - 1 WHERE id = $1', [postId]);
    }
    return { postId, userId, created: false, deleted: del.rowCount === 1 };
  });
}

module.exports = { getPost, likePost, unlikePost, NotFoundError };
