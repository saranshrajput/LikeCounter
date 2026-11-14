const express = require('express');
const service = require('./postLikeService');

const app = express();
app.use(express.json());

const wrap = (fn) => (req, res, next) => Promise.resolve(fn(req, res)).catch(next);

function requireUserId(req, res) {
  const userId = req.body && req.body.userId;
  if (typeof userId !== 'string' || userId.trim() === '') {
    res.status(400).json({ error: 'userId must not be blank' });
    return null;
  }
  return userId;
}

app.get('/health', (req, res) => res.json({ ok: true }));

app.get('/posts/:postId', wrap(async (req, res) => {
  const post = await service.getPost(req.params.postId);
  if (!post) return res.sendStatus(404);
  res.json(post);
}));

app.post('/posts/:postId/like', wrap(async (req, res) => {
  const userId = requireUserId(req, res);
  if (userId === null) return;
  const result = await service.likePost(req.params.postId, userId);
  res.status(result.created ? 201 : 200).json(result);
}));

app.delete('/posts/:postId/like', wrap(async (req, res) => {
  const userId = requireUserId(req, res);
  if (userId === null) return;
  res.json(await service.unlikePost(req.params.postId, userId));
}));

app.use((err, req, res, next) => {
  console.error(err);
  res.status(500).json({ error: 'internal error' });
});

module.exports = app;
