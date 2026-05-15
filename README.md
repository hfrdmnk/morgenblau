# Morgenblau

A calm content platform powered by RSS and ATProto. See [SPEC.md](./SPEC.md) for the product vision and [CLAUDE.md](./CLAUDE.md) for stack conventions.

> Stack: Go API (`cmd/api`) + React/Vite/Tailwind v4 frontend (`frontend/`), Postgres with goose migrations and sqlc-generated queries.

## Prerequisites

- Go (matching `go.mod`)
- [bun](https://bun.sh) ≥ 1.3
- [air](https://github.com/air-verse/air) — Go live-reload
- [mprocs](https://github.com/pvolok/mprocs) — runs the dev processes side-by-side
- [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) — public tunnel for OAuth metadata
- [goose](https://github.com/pressly/goose) and [sqlc](https://sqlc.dev)
- Postgres (e.g. [DBngin](https://dbngin.com))

Install the Go CLIs once:

```sh
go install github.com/air-verse/air@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

## Setup

```sh
cp .env.example .env           # then fill in DB_* and BLUESKY_OAUTH_PRIVATE_KEY
bun install --cwd ./frontend
make migrate-up
```

Generate an OAuth signing key:

```sh
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 | openssl base64 -A
```

## Run

```sh
make dev
```

Starts four processes via `mprocs.yaml`:

- **Server** — `air` rebuilds and runs `cmd/api` on `$PORT` (default `:8000`).
- **Vite** — `bun run --cwd ./frontend dev` on `:5173`.
- **Tunnel** — `cloudflared tunnel run` (see below).
- **Tests** — `go test ./...`, manual-start.

In `APP_ENV=local`, the Go server reverse-proxies `/` to the Vite dev server (HMR works). In any other env it serves `frontend/dist` from the embedded FS (`frontend/embed.go`). Always hit the Go port — same origin keeps `/api/*` cookies and CSRF sane.

If you don't need the tunnel, `make run` starts Go + Vite only.

## Dev OAuth tunnel — one-time setup

Bluesky's authorization server fetches `/oauth-client-metadata.json` over public HTTPS during PAR. We expose `127.0.0.1:$PORT` to the internet via a Cloudflare named tunnel pinned to a stable `<subdomain>.morgen.blue` host.

1. **Install + authenticate** (browser; pick the `morgen.blue` zone):

    ```sh
    brew install cloudflared
    cloudflared tunnel login
    ```

2. **Create a named tunnel** (writes credentials to `~/.cloudflared/<UUID>.json`):

    ```sh
    cloudflared tunnel create morgenblau-<subdomain>
    ```

3. **Route the subdomain** (creates a stable Cloudflare DNS `CNAME`):

    ```sh
    cloudflared tunnel route dns morgenblau-<subdomain> <subdomain>.morgen.blue
    ```

4. **Write `~/.cloudflared/config.yml`** (substitute the UUID from step 2):

    ```yaml
    tunnel: morgenblau-<subdomain>
    credentials-file: /Users/<your-mac-user>/.cloudflared/<UUID>.json

    ingress:
        - hostname: <subdomain>.morgen.blue
          service: http://localhost:8000
        - service: http_status:404
    ```

5. **Set `.env`**:

    ```dotenv
    APP_URL=https://<subdomain>.morgen.blue
    BLUESKY_CLIENT_ID=https://<subdomain>.morgen.blue/oauth-client-metadata.json
    BLUESKY_REDIRECT=https://<subdomain>.morgen.blue/oauth/callback
    ```

The `Tunnel` process in `mprocs.yaml` takes no arguments — it reads `~/.cloudflared/config.yml`, which stays per-machine. The committed config stays dev-agnostic. (`morgen.blue` only has Universal SSL for single-level subdomains; use `<name>.morgen.blue`, not `<name>.<group>.morgen.blue`.)

## Database

Plain SQL, no ORM. Migrations live in `internal/database/migrations/`, handwritten queries in `internal/database/queries/`, generated code in `internal/database/db/`.

```sh
make migrate-up                       # apply pending migrations
make migrate-down                     # roll back one
make migrate-status                   # show applied/pending
make migrate-create NAME=add_users    # scaffold a new SQL migration
make sqlc                             # regenerate queries after edits
```

## Build & test

```sh
make build      # produces ./main (Go binary; in non-local env it serves embedded frontend/dist)
make test       # go test ./... -v
make itest      # integration tests against testcontainers Postgres
```

## Git & PRs

[Tangled](https://tangled.org/dominik.social/morgenblau) is the source of truth; GitHub is a mirror. `origin` pushes to both via a configured second pushurl. **Open PRs on tangled — don't run `gh pr create`.**
