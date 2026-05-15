# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

> **Branch note (`lets-go`):** stack is being rebuilt on Go (backend) + React without Inertia (frontend). Most stack-specific guidance is intentionally absent until the new layout settles.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec.

## Lexicons

`lexicons/` is reference, evolving toward a standardised RSS-reader lexicon for the AT atmosphere. `.json` files are read-only from this codebase. Only `lexicons/app/skyreader/IDEAS.md` is editable — record proposals there before changing schemas.

## Database

Postgres + **goose** (migrations) + **sqlc** (type-safe query codegen). No ORM.

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
