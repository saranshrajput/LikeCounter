const app = require('./app');
const { initDatabase } = require('./db');

const port = process.env.PORT || 3001;

initDatabase()
  .then(() => app.listen(port, () => console.log(`like-counter listening on ${port}`)))
  .catch((err) => {
    console.error(err);
    process.exit(1);
  });
