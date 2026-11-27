# Morgenblau app

The Go backend and React frontend. Run app commands from this directory.

[LAWS.md](./LAWS.md) states the app's invariants. Check it before changing their owning paths.

## Workflow

Write all Go code with Red-Green-TDD. Leverage Go's phenomenal testing suite.
Prove behavior on the running app yourself with the `verify-morgenblau` skill. I do the final visual check and give you feedback.

## Comments

No comments: make the code say it, encode a rule the next change must keep as a test or lint rule, and put a change's reason in its commit message. Only a note about something outside our control stays, and it links the upstream issue or docs. `TestCommentsStayWithinBaseline` in `internal/comments` and `frontend/eslint/comments.js` enforce it and ban `//nolint` and `eslint-disable`; their failure messages say what to do. The baselines, `comments-baseline.json` and `frontend/eslint-suppressions.json`, only shrink: run `make comments-prune` after deleting baselined comments.

## Lexicons

Schemas and their rules: [lexicons/AGENTS.md](lexicons/AGENTS.md). `SPEC.md` defines the compatibility boundary.

## Database

SQLite through the pure-Go `modernc.org/sqlite`, so `make build-linux` builds with `CGO_ENABLED=0`; goose for migrations, sqlc for queries, no ORM. The rules live in [internal/database/AGENTS.md](internal/database/AGENTS.md). `Open` in `internal/database/conn.go` owns the DB path (`DB_PATH`) and the pragmas; the `Makefile` owns the targets.

Install goose, sqlc and golangci-lint once, at the versions `.github/workflows/ci.yml` pins; a different sqlc version rewrites the generated code's header and fails CI's `sqlc diff`.

## Deploy

Pushes to `main` deploy to Fly from the `deploy` job in `.github/workflows/ci.yml`, only after CI passes for that commit. [DEPLOY.md](DEPLOY.md) owns the production setup.

## ATProto

Protocol work starts from the project's own sources: `SPEC.md` for the data model and ownership, [lexicons/AGENTS.md](lexicons/AGENTS.md) for schemas, and the SSRF and PDS-mutation rules in [internal/AGENTS.md](internal/AGENTS.md). For protocol details they don't cover, read the documentation at [atproto.com/docs](https://atproto.com/docs).

## atproto.md

For read-only lookups of public ATProto data, use [atproto.md](https://atproto.md). Its endpoint index is [atproto.md/llms.txt](https://atproto.md/llms.txt).
