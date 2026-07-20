---
paths:
  - "internal/database/**"
  - "sqlc.yaml"
---

# Database conventions

- `internal/database/db/` is sqlc-generated. Never edit it by hand; change `queries/` or the migration schema, run `make sqlc`, commit the regenerated code.
- Migrations are plain SQL in `migrations/` with `-- +goose Up` / `-- +goose Down` markers, applied only via the goose CLI (`make migrate-*`), never in-process. Every migration needs a working Down.
- Queries are handwritten SQL in `queries/` annotated `-- name: FuncName :one|:many|:exec`. Ownership is enforced in SQL: user-scoped queries filter by `did` so handlers never fetch-then-compare.
- Two pools over one SQLite file: writer (single connection, `_txlock=immediate`) and reader. Wire consumers to the correct pool; multi-statement write batches go through the `database` package's Tx helper. Never call non-Tx writer queries from inside an open transaction, and keep network I/O outside transactions.
- Sensitive blobs (tokens, session material) are AEAD-encrypted at the serializer before storage; see the Go rules.
- Storage tests run against real SQLite in `t.TempDir()`, never `:memory:` when more than one pool is involved.
