const { pool } = require('./db');

async function getPost(postId) {
  const { rows } = await pool.query(
    'SELECT id, title, like_count FROM posts WHERE id = $1',
    [postId]
  );
  if (rows.length === 0) return null;
  const r = rows[0];
  return { id: r.id, title: r.title, likeCount: r.like_count };
}

// NOTE: intentionally naive baseline (check-then-insert, no transaction).
async function likePost(postId, userId) {
  const { rows } = await pool.query(
    'SELECT COUNT(*)::int AS c FROM post_likes WHERE post_id = $1 AND user_id = $2',
    [postId, userId]
  );
  if (rows[0].c > 0) {
    return { postId, userId, created: false, deleted: false };
  }

  await pool.query('INSERT INTO post_likes (post_id, user_id) VALUES ($1, $2)', [postId, userId]);
  await pool.query('UPDATE posts SET like_count = like_count + 1 WHERE id = $1', [postId]);

  return { postId, userId, created: true, deleted: false };
}

// NOTE: intentionally naive baseline (always decrements).
async function unlikePost(postId, userId) {
  await pool.query('DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2', [postId, userId]);
  await pool.query('UPDATE posts SET like_count = like_count - 1 WHERE id = $1', [postId]);

  return { postId, userId, created: false, deleted: true };
}

module.exports = { getPost, likePost, unlikePost };
