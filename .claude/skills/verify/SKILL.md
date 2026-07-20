---
name: verify
description: Build, launch, and drive Morgenblau (Go API + Vite frontend) to verify changes at runtime.
---

# Verifying Morgenblau at runtime

## Launch

- Backend: `go run cmd/api/main.go` (reads `.env`; `PORT=8000`, `DB_PATH=./data/morgenblau.db`). Startup log shows feed fetch + discover trending batch; the batch runs immediately and exercises the discovercrawl paths against the real network.
- Frontend dev: `bun run --cwd frontend dev` (vite, proxies `/api` to the Go port). Combined: `make dev` needs mprocs + air.
- Production bundle over HTTP: `cd frontend && bun run build && bunx vite preview --port 4173`.

## Drive

- Almost every `/api/*` route sits behind session auth; unauthed requests get a plain-text 401 from the middleware, not the JSON `{code,message}` contract. There is no dev bypass; authed flows need a real browser session.
- Public-ish surfaces: the built shell at `vite preview`, chunk loading (page chunks are separate assets referenced from the shell), and the API's 401 behavior.
- Backend behavior often observable via the startup batch log (`discovercrawl:` warns) and the SQLite DB directly: `sqlite3 data/morgenblau.db` (tables like `discover_trending_signals`, `discover_crawl_authored`).

## Gotchas

- `/` on the Go port returns 502 unless vite dev is running (dev proxy).
- The dev DB is real state; goose only runs migrations forward, so editing an already-applied migration desyncs it (fix with a manual ALTER or delete the DB file).
- Kill test servers when done: `pkill -f "go run cmd/api/main.go"; pkill -f "vite preview"`.
