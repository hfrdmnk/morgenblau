---
paths:
  - "**/*.go"
---

# Go conventions

## Structure

Review source layout and dependency wiring against these conventions:

- Binaries live in `cmd/`; reusable application behavior lives in `internal/` packages organized by feature. Developer tools may own their tool-specific setup and orchestration in `cmd/`. Packages stay small, single-purpose, and leaf-like. Only `server`, `api`, and `sync` fan out to many imports.
- `internal/server.NewServer()` is the application's composition root. Developer tools compose their own dependencies; neither uses package-level state or a service container for wiring.
- Consumers define the interfaces they need, as narrow as their actual call surface. Accept interfaces, return structs. Concrete types come from constructors or sqlc (`internal/database/db`).
- stdlib first: routing, JSON, and middleware stay on `net/http`. A new third-party dependency needs explicit justification.
- Review goroutine ownership: every worker is cancelled and awaited on shutdown. `internal/server/shutdown_test.go` owns the server's drain-before-storage-close checks. No fire-and-forget goroutines from handlers.
- CI runs gofmt, `go vet` and golangci-lint; `.golangci.yml` holds the repo's bans, each with its reason.

## Global rules (error classes to prevent)

- **SSRF:** fetch attacker-influenceable URLs and resolve handles/DIDs through `internal/safehttp` or `internal/atidentity`. Review library-client defaults for guarded HTTP and identity wiring; `.golangci.yml` and `TestNewOAuthApp_InstallsGuardedClientAndDir` own the automated construction checks.
- **Transactions:** `database.Open` owns the pools; `database.WithTx` owns write batches and the writer-connection constraint. Wire handlers and jobs to the correct pool, and do all network I/O before the transaction opens.
- **Encryption:** tokens, keys, and session material are AEAD-encrypted before they touch the database. Keys come from env and support rotation: the first key encrypts, all keys are tried on decrypt. Never persist credentials in plaintext.
- **HTTP surface:** use the helpers in `internal/api/respond.go`; `TestAPIErrorsUseTheRespondEnvelope` owns the error-contract checks. `internal/middleware/auth/` owns `/api/` body limits. Missing-or-not-owned resources return 404 on every verb; only reauth uses 403 + `reauth_required`. OAuth HTML-flow handlers are the only plain-text error exception.

## PDS mutations

- Dedupe and validate against the lexicon before the commit. Law 1 in `LAWS.md` owns commit-then-mirror and the one-commit budget; `commitThenMirror` and `CommitGate` in `internal/api/` implement it.
- Outbound atproto HTTP is built by `atxrpc.New`, which installs a per-host cooldown honoring `Retry-After` and rate-limit headers. New fetch loops inherit it by construction; never hand-roll retries or retry-hint parsing at a call site.

## Testing

- Red-Green TDD: write the failing test first. Handlers use hand-rolled fakes of their narrow interfaces with `httptest`. Storage fixtures follow `TestStorageTestsUseAFileNotMemory`; concurrent code runs under `-race`.
- Fixtures follow the Testing rule in the root `AGENTS.md`.
