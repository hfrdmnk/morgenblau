# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

> **Branch note (`lets-go`):** stack is being rebuilt on Go (backend) + React without Inertia (frontend). Most stack-specific guidance is intentionally absent until the new layout settles.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec.

## Workflow

Write all Go code with Red-Green-TDD. Leverage Go's phenomenal testing suite.

## Lexicons

Lexicon schemas live in `SPEC.md` under `<lexicons>`. Morgenblau owns `blue.morgen.*`; external lexicons we interoperate with (Bluesky, margin.at, Glean, Skyreader) are listed in the same section.

## Database

SQLite (pure-Go `modernc.org/sqlite`) + **goose** (migrations) + **sqlc** (type-safe query codegen). No ORM.

- DB file: `./data/morgenblau.db` (override with `DB_PATH`). The dir is created on first open and gitignored.
- Pragmas (set in DSN): `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=on`, `synchronous=normal`.
- Pure-Go driver so `make build-linux` (`CGO_ENABLED=0`) keeps working.
- Migrations: `internal/database/migrations/` — plain SQL with `-- +goose Up` / `-- +goose Down` markers.
- Queries: `internal/database/queries/` — handwritten SQL annotated with `-- name: FuncName :one|:many|:exec`.
- Generated code: `internal/database/db/` — committed; regenerate with `make sqlc` after editing queries or schema.
- Make targets: `migrate-up`, `migrate-down`, `migrate-status`, `migrate-create NAME=...`, `sqlc`.

Install CLIs once: `go install github.com/pressly/goose/v3/cmd/goose@latest && go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`.

## Git & PRs

- **Tangled is the source of truth, GitHub is a mirror.** `origin` points at `git@tangled.org:dominik.social/morgenblau`; `git push` auto-mirrors to GitHub via a second pushurl.
- **Open PRs on tangled, not GitHub.** Don't run `gh pr create`.

## ATProto Skills

For ATProto protocol work, invoke the matching skill: `atproto-oauth`, `atproto-lexicon`, `atproto-publish-lexicon`, `atproto-identity-resolution`, `atproto-repository`, `atproto-cid`, `atproto-attestation`.
