---
paths:
  - "**/*.go"
---

# Go conventions

## Structure

- Binaries live in `cmd/`; all logic lives in `internal/` packages organized by feature. Packages stay small, single-purpose, and leaf-like. Only `server`, `api`, and `sync` fan out to many imports.
- `internal/server.NewServer()` is the sole composition root: every dependency is wired there by hand, never through package-level state or a service container.
- Consumers define the interfaces they need, as narrow as their actual call surface. Accept interfaces, return structs. Concrete types come from constructors or sqlc (`internal/database/db`).
- stdlib first: routing, JSON, and middleware stay on `net/http`. A new third-party dependency needs explicit justification.
- Every goroutine has an owned lifecycle: context cancellation plus a WaitGroup, drained on shutdown. No fire-and-forget goroutines from handlers.
- CI runs gofmt, `go vet` and golangci-lint; `.golangci.yml` holds the repo's bans, each with its reason.

## Global rules (error classes to prevent)

- **SSRF:** any client that fetches attacker-influenceable URLs or resolves handles/DIDs goes through `internal/safehttp` or `internal/atidentity`. When constructing a library client, audit its defaults and override its HTTP client and identity directory.
- **Transactions:** SQLite has one writer pool (single connection) and one reader pool; wire handlers and jobs to the correct one. Multi-statement write batches run in one transaction via the `database` package's Tx helper. Never touch the non-transaction writer queries from inside an open transaction, and do all network I/O before the transaction opens.
- **Encryption:** tokens, keys, and session material are AEAD-encrypted before they touch the database. Keys come from env and support rotation: the first key encrypts, all keys are tried on decrypt. Never persist credentials in plaintext.
- **HTTP surface:** the auth middleware size-limits `/api/` bodies; decode with `api.decodeJSON`. Handlers in `internal/api` write errors only through `writeError` with a `respond.go` code, or `writeFieldErrors` for a form's field map (`TestAPIErrorsUseTheRespondEnvelope`). Unrouted `/api/` requests and the auth middleware's 401 answer with the same `{code, message}` envelope; the OAuth HTML-flow handlers are the only plain-text exception. Missing-or-not-owned resources return 404 on every verb; only the reauth contract uses 403 + `reauth_required`.

## PDS mutations

- Dedupe and validate against the lexicon before the commit. Law 1 in `LAWS.md` owns commit-then-mirror and the one-commit budget; `commitThenMirror` and `CommitGate` in `internal/api/` implement it.
- Outbound atproto HTTP is built by `atxrpc.New`, which installs a per-host cooldown honoring `Retry-After` and rate-limit headers. New fetch loops inherit it by construction; never hand-roll retries or retry-hint parsing at a call site.

## Testing

- Red-Green TDD: write the failing test first. Handlers test against hand-rolled fakes of their own narrow interfaces with `httptest`; storage tests open a real SQLite file in `t.TempDir()` (`TestStorageTestsUseAFileNotMemory`). Concurrent code runs under `-race`.
- Fixtures follow the Testing rule in the root `AGENTS.md`.
