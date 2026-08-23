# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

> **Branch note (`lets-go`):** stack is being rebuilt on Go (backend) + React without Inertia (frontend). Most stack-specific guidance is intentionally absent until the new layout settles.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec.

## Workflow

Write all Go code with Red-Green-TDD. Leverage Go's phenomenal testing suite.
Don't try to navigate the app in the browser yourself. I will always check myself and give you feedback.

## Comments

Comment the why, never the what. A comment earns its place only when it records something the code cannot express: a non-obvious constraint, an ordering requirement, a protocol quirk, a deliberate trade-off. Never restate the next line, narrate a change, or justify a diff. If code needs explaining, rewrite the code first. One line by default. Applies to all languages in this repo.

## Docs

Docs point, they don't mirror. In durable project docs (this file, `SPEC.md`, `.claude/rules/*`, memory), write pointers and invariants, not a copy of what the code already owns. Name the file or function that holds the detail and state the rule; don't enumerate a list the code will grow past (an error-code set, a route table).

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

- **GitHub is the source of truth, Tangled is a mirror.** `origin` points at `git@github.com:hfrdmnk/morgenblau.git`; `git push` auto-mirrors to Tangled via a second pushurl.
- **Open PRs on GitHub.**

## ATProto Skills

For ATProto protocol work, invoke the matching skill: `atproto-oauth`, `atproto-lexicon`, `atproto-publish-lexicon`, `atproto-identity-resolution`, `atproto-repository`, `atproto-cid`, `atproto-attestation`.

## atproto.md

> A read-only, markdown-first API for the AT Protocol ecosystem. Returns structured markdown from any PDS. Accepts at:// URIs directly in the URL path. No authentication required — public data only.

atproto.md resolves handles and DIDs, fetches data directly from the user's PDS via `com.atproto.repo.*`, and returns rich markdown. Works with any collection on any PDS — not just Bluesky.

### Endpoints

- [Resolve identity](https://atproto.md/resolve/{handle-or-did}): Full identity chain — handle → DID → DID document → PDS endpoint
- [PLC audit log](https://atproto.md/plc/audit/{handle-or-did}): Chronological history of a did:plc identity — PDS migrations, handle changes, key rotations
- [PLC data](https://atproto.md/plc/data/{handle-or-did}): Current canonical PLC state — active PDS, handles, signing key, and rotation keys
- [PLC last op](https://atproto.md/plc/last/{handle-or-did}): The most recent PLC operation and the state it established
- [Usage stats](https://atproto.md/stats): Anonymous traffic — route + MCP-tool hit counts and most-queried collections. No user-specific data
- [Repo overview](https://atproto.md/at://{actor}): Lists all collections in an actor's repo
- [List records](https://atproto.md/at://{actor}/{collection}): Paginated records from any collection. Params: limit (default 25, max 100), cursor, reverse
- [Get record](https://atproto.md/at://{actor}/{collection}/{rkey}): Fetch a single record by its rkey
- [Get lexicon](https://atproto.md/lexicon/{nsid}): Resolve a Lexicon schema by NSID via DNS-based lexicon resolution (_lexicon TXT → DID → com.atproto.lexicon.schema record)
- [Discover repos by collection](https://atproto.md/discover/{collection}): Every repo on the network with records in a collection NSID. Params: limit (default 100, max 2000), cursor
- [Backlinks](https://atproto.md/backlinks/{at-uri-or-did-or-url}): Who links to a target (likes, reposts, replies, follows, any lexicon). Summary of sources by default; add source={collection:path} to list linking records

### Examples

- [Resolve bsky.app](https://atproto.md/resolve/bsky.app)
- [PLC audit log for bsky.app](https://atproto.md/plc/audit/bsky.app)
- [PLC data for bsky.app](https://atproto.md/plc/data/bsky.app)
- [Browse repo](https://atproto.md/at://bsky.app)
- [List posts](https://atproto.md/at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post?limit=5)
- [Get profile](https://atproto.md/at://bsky.app/app.bsky.actor.profile/self)
- [Get the app.bsky.feed.post lexicon](https://atproto.md/lexicon/app.bsky.feed.post)
- [Discover site.standard.document repos](https://atproto.md/discover/site.standard.document)
- [Backlinks to a post](https://atproto.md/backlinks/at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3lgwdn7vd722r)
