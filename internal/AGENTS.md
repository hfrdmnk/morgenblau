---
paths:
  - "**/*.go"
---

# Go conventions

## Structure

- One binary (`cmd/api`); all logic lives in `internal/` packages organized by feature. Packages stay small, single-purpose, and leaf-like. Only `server`, `api`, and `sync` fan out to many imports.
- `internal/server.NewServer()` is the sole composition root: every dependency is wired there by hand. No globals, no `init()` wiring, no service containers.
- Consumers define the interfaces they need, as narrow as their actual call surface. Accept interfaces, return structs. Concrete types come from constructors or sqlc (`internal/database/db`).
- stdlib first: routing, JSON, and middleware stay on `net/http`. A new third-party dependency needs explicit justification.
- `log/slog` for all logging. Code must be gofmt-clean and `go vet`-clean.
- Every goroutine has an owned lifecycle: context cancellation plus a WaitGroup, drained on shutdown. No fire-and-forget goroutines from handlers.

## Global rules (error classes to prevent)

- **SSRF:** any client that fetches attacker-influenceable URLs or resolves handles/DIDs goes through `internal/safehttp` or `internal/atidentity`. Never `http.DefaultClient`. When constructing a library client, audit its defaults and override its HTTP client and identity directory.
- **Transactions:** SQLite has one writer pool (single connection) and one reader pool; wire handlers and jobs to the correct one. Multi-statement write batches run in one transaction via the `database` package's Tx helper. Never touch the non-transaction writer queries from inside an open transaction, and do all network I/O before the transaction opens.
- **Encryption:** tokens, keys, and session material are AEAD-encrypted before they touch the database. Keys come from env and support rotation: the first key encrypts, all keys are tried on decrypt. Never persist credentials in plaintext.
- **HTTP surface:** request bodies are size-limited via the shared middleware; decode with `api.decodeJSON` (413 on overflow, 400 on malformed). Every JSON error body goes through `writeError(w, status, code, msg)` or `writeFieldErrors(w, fields)` in `internal/api/respond.go`. Never `http.Error` or an ad-hoc `{"message": ...}` map in an `internal/api` handler (the OAuth HTML-flow handlers are the only `http.Error` exception). `code` is a stable slug the frontend keys off, defined in `respond.go`; add one there rather than inventing a body shape. Missing-or-not-owned resources return 404 on every verb; only the reauth contract uses 403 + `reauth_required`.

## PDS mutations

- Order is fixed: dedupe, validate against the lexicon, write to the PDS, then mirror into the local index. Never mirror first; the PDS is the authority and the local table is a derived index.
- The mirror write goes through `mirrorOrRepair` (`internal/api/mirror.go`) and never fails the response: the PDS write it follows already committed, so a failed mirror dispatches a repair sync instead of surfacing an error.
- Outbound atproto HTTP is built by `atxrpc.New`, which installs a per-host cooldown honoring `Retry-After` and rate-limit headers. New fetch loops inherit it by construction; never hand-roll retries or retry-hint parsing at a call site.

## Testing

- Red-Green TDD: write the failing test first. Handlers test against hand-rolled fakes of their own narrow interfaces with `httptest`; storage tests use real SQLite in `t.TempDir()`. Concurrent code runs under `-race`.
- Generic fixtures only. Test data never references real people, handles, domains, or publication names, even ones encountered while researching a feature. Use reserved namespaces: `example.com` subdomains for hosts, `*.example` for handles, invented names like "Example Publication" for record fields. Real-world observations belong in research notes or SPEC.md, not in fixtures.
