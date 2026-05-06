#!/usr/bin/env bash
# Preseed for local-auto-adopt-node-postgres-first-deploy.
#
# Writes a minimal Node + Postgres notes API into the eval workdir,
# so the agent's "deploy this folder" instruction has real code to
# push. Runs in the workdir that the Runner sets up (CWD when the
# script executes).
#
# Files written:
#   package.json   — pg + express deps (no scaffolding tools)
#   server.js      — minimal API reading ${db_*} env vars
#   .gitignore     — node_modules + .env (so push is small)
#   zerops.yml     — single 'app' setup, nodejs@22 runtime, port 3000
#
# Intentionally does NOT write .env (the agent should generate it via
# zerops_env action="generate-dotenv"). Intentionally does NOT
# pre-install node_modules (build runs `npm install` on Zerops).

set -euo pipefail
cd "${1:-.}"

cat >package.json <<'JSON'
{
  "name": "team-notes",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "start": "node server.js"
  },
  "dependencies": {
    "express": "^4.19.2",
    "pg": "^8.11.5"
  }
}
JSON

cat >server.js <<'JS'
import express from "express";
import pg from "pg";

const port = process.env.PORT || 3000;
const pool = new pg.Pool({
  host: process.env.db_host,
  port: process.env.db_port,
  user: process.env.db_user,
  password: process.env.db_password,
  database: process.env.db_dbName,
});

const app = express();
app.use(express.json());

app.get("/health", async (_req, res) => {
  try {
    await pool.query("SELECT 1");
    res.json({ status: "ok" });
  } catch (e) {
    res.status(500).json({ status: "db-unreachable", error: String(e) });
  }
});

app.get("/notes", async (_req, res) => {
  const r = await pool.query("SELECT id, body FROM notes ORDER BY id DESC");
  res.json(r.rows);
});

app.post("/notes", async (req, res) => {
  const r = await pool.query(
    "INSERT INTO notes(body) VALUES($1) RETURNING id, body",
    [req.body.body ?? ""]
  );
  res.json(r.rows[0]);
});

app.listen(port, () => console.log(`team-notes listening on :${port}`));
JS

cat >.gitignore <<'GITIGNORE'
node_modules/
.env
GITIGNORE

cat >zerops.yml <<'YML'
zerops:
  - setup: app
    build:
      base: nodejs@22
      buildCommands:
        - npm ci
      deployFiles: ./
      cache:
        - node_modules
    run:
      base: nodejs@22
      ports:
        - port: 3000
          httpSupport: true
      start: npm start
      initCommands:
        - psql "$db_connectionString" -c "CREATE TABLE IF NOT EXISTS notes (id SERIAL PRIMARY KEY, body TEXT NOT NULL)"
YML
