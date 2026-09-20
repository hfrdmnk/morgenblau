---
name: verify
description: Build, launch, and drive Morgenblau (Go API + Vite frontend) to verify changes at runtime.
---

# Verifying Morgenblau at runtime

## Launch

- Backend: `go run cmd/api/main.go` (reads `.env`; `PORT=8000`, `DB_PATH=./data/morgenblau.db`). Startup wiring and the feed refresh loop live in `internal/server/server.go`; no discovery ingestion runs.
- Frontend dev: `bun run --cwd frontend dev` (vite, proxies `/api` to the Go port). Combined: `make dev` needs mprocs + air.
- Production bundle over HTTP: `cd frontend && bun run build && bunx vite preview --port 4173`.

## Drive

- Almost every `/api/*` route sits behind session auth; unauthed requests get a plain-text 401 from the middleware, not the JSON `{code,message}` contract. There is no dev bypass; authed flows need a real browser session.
- Public-ish surfaces: the built shell at `vite preview`, chunk loading (page chunks are separate assets referenced from the shell), and the API's 401 behavior.
- Inspect backend logs and the SQLite DB for retained feed and subscription behavior. Table definitions live in `internal/database/migrations/`.

## Gotchas

- `/` on the Go port returns 502 unless vite dev is running (dev proxy).
- The v1 simplification uses a fresh migration baseline. For an older local checkout's database, stop the server, select a new `DB_PATH`, and apply migrations as described in `README.md`.
- Kill test servers when done: `pkill -f "go run cmd/api/main.go"; pkill -f "vite preview"`.
