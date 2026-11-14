const fs = require('node:fs');
const path = require('node:path');
const { Pool } = require('pg');

const pool = new Pool({
  connectionString:
    process.env.DATABASE_URL ||
    'postgresql://like_counter:like_counter@localhost:5438/like_counter',
  max: 10, // same as Hikari's default
});

// Mirrors spring.sql.init.mode=always: run schema.sql then data.sql on startup.
async function initDatabase({ seed = true } = {}) {
  const run = (f) => fs.readFileSync(path.join(__dirname, '..', 'sql', f), 'utf8');
  await pool.query(run('schema.sql'));
  if (seed) await pool.query(run('data.sql'));
}

module.exports = { pool, initDatabase };
