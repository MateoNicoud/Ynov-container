const os = require('os');
const express = require('express');
const { Pool } = require('pg');

const pool = new Pool({
  host: process.env.DB_HOST || 'db',
  port: 5432,
  user: process.env.POSTGRES_USER,
  password: process.env.POSTGRES_PASSWORD,
  database: process.env.POSTGRES_DB,
});

const COLORS = ['yellow', 'pink', 'blue', 'green'];
const app = express();
app.use(express.json({ limit: '10kb' }));

const wrap = (fn) => (req, res) =>
  fn(req, res).catch((err) => {
    console.error(err);
    res.status(500).json({ error: 'erreur serveur' });
  });

app.get('/health', (req, res) => res.send('ok'));

app.get('/info', wrap(async (req, res) => {
  const r = await pool.query(
    'SELECT host(inet_server_addr()) AS db_ip, (SELECT count(*) FROM notes)::int AS total'
  );
  res.json({ api_host: os.hostname(), db_ip: r.rows[0].db_ip, total: r.rows[0].total });
}));

app.get('/notes', wrap(async (req, res) => {
  const r = await pool.query(
    'SELECT id, author, message, color, created_at FROM notes ORDER BY id DESC LIMIT 60'
  );
  res.json(r.rows);
}));

app.post('/notes', wrap(async (req, res) => {
  const author = String(req.body.author || '').trim().slice(0, 30) || 'Anonyme';
  const message = String(req.body.message || '').trim().slice(0, 140);
  const color = COLORS.includes(req.body.color) ? req.body.color : 'yellow';
  if (!message) return res.status(400).json({ error: 'message vide' });
  const r = await pool.query(
    'INSERT INTO notes (author, message, color) VALUES ($1, $2, $3) RETURNING id, author, message, color, created_at',
    [author, message, color]
  );
  res.status(201).json(r.rows[0]);
}));

app.delete('/notes/:id', wrap(async (req, res) => {
  const id = Number(req.params.id);
  if (!Number.isInteger(id)) return res.status(400).json({ error: 'id invalide' });
  await pool.query('DELETE FROM notes WHERE id = $1', [id]);
  res.status(204).end();
}));

async function init() {
  for (let i = 0; i < 20; i++) {
    try {
      await pool.query(`CREATE TABLE IF NOT EXISTS notes (
        id serial PRIMARY KEY,
        author text NOT NULL,
        message text NOT NULL,
        color text NOT NULL DEFAULT 'yellow',
        created_at timestamptz NOT NULL DEFAULT now()
      )`);
      return;
    } catch (e) {
      console.log('BDD pas prête, nouvel essai...');
      await new Promise((r) => setTimeout(r, 1500));
    }
  }
  console.error('Impossible de joindre la BDD');
  process.exit(1);
}

init().then(() => app.listen(3000, () => console.log('API prête sur :3000')));
