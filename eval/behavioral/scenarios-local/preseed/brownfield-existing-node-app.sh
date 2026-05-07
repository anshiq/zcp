#!/usr/bin/env bash
# Preseed for brownfield-existing-node-app scenario.
#
# Writes a working Node + Express + Postgres app with a populated
# `.env` pointing at LOCAL Postgres (not Zerops). Agent must classify
# the `.env` entries and propose a managed Postgres + redistribute
# the entries across project envVariables / zerops.yaml run.envVars
# / .env.local for local-only flags.
#
# Files written:
#   package.json — express + pg
#   server.js    — minimal API reading process.env.DATABASE_URL
#   .gitignore   — excludes node_modules + .env
#   .env         — POPULATED with localhost DB + secrets + mode flags

set -euo pipefail
cd "${1:-.}"

cat >package.json <<'JSON'
{
  "name": "team-notes-existing",
  "version": "0.2.0",
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
const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL });

const app = express();
app.use(express.json());

app.get("/health", async (_req, res) => {
  try {
    await pool.query("SELECT 1");
    res.json({ status: "ok", env: process.env.NODE_ENV });
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
.env.local
GITIGNORE

# The point of this fixture: a populated .env with mixed-channel
# entries the agent must redistribute. Localhost DB, shared secrets,
# env-mode flag, app config — all the categories the design's
# classification heuristic addresses.
cat >.env <<'ENV'
DATABASE_URL=postgresql://devuser:devpass@localhost:5432/teamnotes
JWT_SECRET=existing-jwt-secret-do-not-rotate-without-warning
SESSION_SECRET=existing-session-secret-also-keep
NODE_ENV=development
LOG_LEVEL=debug
PORT=3000
APP_REQUEST_TIMEOUT=30000
ENV

echo "preseed: brownfield app + populated .env in $(pwd)"
