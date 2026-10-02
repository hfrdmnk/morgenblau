# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

> Stack: Go backend and React frontend. The v1 boundary is defined in `SPEC.md`.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec.

[LAWS.md](./LAWS.md) states the repo-wide invariants. Check it before changing their owning paths.

## Workflow

Write all Go code with Red-Green-TDD. Leverage Go's phenomenal testing suite.
Prove behavior on the running app yourself with the `verify-morgenblau` skill. I do the final visual check and give you feedback.

## Comments

Comment the why, never the what. A comment earns its place only when it records something the code cannot express: a non-obvious constraint, an ordering requirement, a protocol quirk, a deliberate trade-off. Never restate the next line, narrate a change, or justify a diff. If code needs explaining, rewrite the code first. One line by default. Applies to all languages in this repo.

## Docs

Docs point, they don't mirror. In durable project docs (this file, `SPEC.md`, `.claude/rules/*`, memory), write pointers and invariants, not a copy of what the code already owns. Name the file or function that holds the detail and state the rule; don't enumerate a list the code will grow past (an error-code set, a route table).

## Testing

Generic fixtures only: test data never references real people, handles, domains, or publications. Use `example.com` subdomains for hosts, `*.example` for handles, invented names like "Example Publication", and placeholder DIDs. Real-world observations belong in research notes or `SPEC.md`.

## Lexicons

Schemas and their rules: [lexicons/AGENTS.md](lexicons/AGENTS.md). `SPEC.md` defines the compatibility boundary.

## Database

SQLite through the pure-Go `modernc.org/sqlite`, so `make build-linux` builds with `CGO_ENABLED=0`; goose for migrations, sqlc for queries, no ORM. The rules live in [internal/database/AGENTS.md](internal/database/AGENTS.md). `Open` in `internal/database/conn.go` owns the DB path (`DB_PATH`) and the pragmas; the `Makefile` owns the targets.

Install goose, sqlc and golangci-lint once, at the versions `.github/workflows/ci.yml` pins; a different sqlc version rewrites the generated code's header and fails CI's `sqlc diff`.

## Git & PRs

- **GitHub is the source of truth, Tangled is a read-only mirror.** Ship to GitHub's `main`; `.github/workflows/sync-tangled.yml` force-syncs accepted pushes to Tangled.
- `scripts/sync-tangled.sh` is the manual recovery path when the mirror workflow needs to be retried outside GitHub.
- Never bypass repository Git hooks with `--no-verify` unless the user explicitly approves it for that invocation. Investigate and resolve hook failures instead.

## ATProto Skills

For ATProto protocol work, invoke the matching `atproto-*` skill.

## atproto.md

For read-only lookups of public ATProto data, use [atproto.md](https://atproto.md). Its endpoint index is [atproto.md/llms.txt](https://atproto.md/llms.txt).
