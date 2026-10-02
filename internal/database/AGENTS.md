---
paths:
  - "internal/database/**"
  - "sqlc.yaml"
---

# Database conventions

- `internal/database/db/` is sqlc-generated; CI's `sqlc diff` fails on hand edits and stale output. Change `queries/` or the migration schema, run `make sqlc`, commit the regenerated code.
- Migrations are plain SQL in `migrations/` with `-- +goose Up` / `-- +goose Down` markers, applied only through the goose CLI (`make migrate-*`; `.golangci.yml` bans importing goose). `TestEveryMigrationRollsBackCleanly` requires a working Down.
- Queries are handwritten SQL in `queries/` annotated `-- name: FuncName :one|:many|:exec`. Ownership is enforced in SQL: user-scoped queries filter by `did` so handlers never fetch-then-compare.
- `Open` in `conn.go` opens the writer and reader pools; the Transactions and Encryption rules in [internal/AGENTS.md](../AGENTS.md) say how to use them.
