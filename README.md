# Morgenblau

A personal daily newspaper powered by RSS and ATProto. See [SPEC.md](./SPEC.md) for the v1 boundary and [AGENTS.md](./AGENTS.md) for stack conventions.

> Stack: Go API (`cmd/api`) + React/Vite/Tailwind v4 frontend (`frontend/`), SQLite with goose migrations and sqlc-generated queries.

This branch simplifies the existing reader by removing social features and discovery. It does not yet implement every v1 requirement; the boundary between retained behavior and future work lives in `SPEC.md`.

## Prerequisites

- Go (matching `go.mod`)
- [bun](https://bun.sh) ≥ 1.3
- [air](https://github.com/air-verse/air) — Go live-reload
- [mprocs](https://github.com/pvolok/mprocs) — runs the dev processes side-by-side
- [goose](https://github.com/pressly/goose) and [sqlc](https://sqlc.dev)

Install the Go CLIs once:

```sh
go install github.com/air-verse/air@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

## Setup

```sh
cp .env.example .env           # fill in signing and session keys documented there
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

Starts processes via `mprocs.yaml`:

- **Server** — `air` rebuilds and runs `cmd/api` on `$PORT` (default `:8000`). *Autostart.*
- **Vite** — `bun run --cwd ./frontend dev` on `:5173`. *Autostart.*
- **Tests** — `go test ./...`. *Manual-start.*

In `APP_ENV=local`, the Go server reverse-proxies `/` to the Vite dev server (HMR works). In any other env it serves `frontend/dist` from the embedded FS (`frontend/embed.go`). Open **`http://127.0.0.1:8000`** — same origin keeps `/api/*` cookies and CSRF sane, and the loopback origin doubles as your OAuth `client_id`.

`make run` starts Go + Vite without `mprocs` if you'd rather use your own terminal layout.

### Inbound newsletters

Inbound email stays disabled when `NEWSLETTER_DOMAIN` and `SMTP_LISTEN_ADDR` are empty. To exercise it locally, set a private test domain and an unprivileged listen address such as `:2525`. The HTTP server and SMTP receiver run in the same process and use the same SQLite database.

STARTTLS is configured with either `SMTP_TLS_CERT_FILE` plus `SMTP_TLS_KEY_FILE`, or the base64 PEM environment variables documented in `.env.example`. `SMTP_HOSTNAME` is the SMTP banner and MX target hostname; it defaults to `NEWSLETTER_DOMAIN`. Production startup verifies the certificate's validity period and SAN against that hostname.

For a local plaintext receiver, add these values to `.env`, start the normal app, sign in, and copy your private address from the Newsletters screen:

```sh
NEWSLETTER_DOMAIN=newsletter.localhost
SMTP_LISTEN_ADDR=127.0.0.1:2525
```

Send the built-in MIME fixture to that copied address. It includes HTML, one embedded CID image, and one remote image placeholder for checking the consent flow:

```sh
go run ./cmd/send-newsletter -to '<your-copied-address>'
```

To exercise forwarded or unusual mail, pass an existing message with `-eml ./message.eml`. The sender defaults to `127.0.0.1:2525` and never contacts an external SMTP server unless `-smtp` is explicitly changed.

## OAuth

Local dev uses a **loopback client**: `client_id` is `http://localhost`, callback is `http://127.0.0.1:8000/oauth/callback`, and the AS skips the client-metadata fetch entirely. Leave `BLUESKY_CLIENT_ID` and `BLUESKY_REDIRECT` empty in `.env` and sign in straight from `http://127.0.0.1:8000` — no tunnel, no public hostname.

For prod, set both env vars to your public URLs and serve `oauth-client-metadata.json` + `oauth-jwks.json` from that origin.

### Scopes

The retained OAuth scopes are configured in `.env.example` and checked in `internal/oauth/scopes/`. Standardfeed subscription writes require their own grant. Sessions without it still work for RSS, but a Standardfeed write prompts a fresh sign-in. The existing Morgenblau permission set stays unchanged for lexicon compatibility; sharing and following have no runtime feature in v1.

## Database

SQLite via the pure-Go `modernc.org/sqlite` driver — the DB file lives at `$DB_PATH` (default `./data/morgenblau.db`) and is created on first open. WAL mode, foreign keys on, and a 5s busy timeout are set via DSN pragmas. Plain SQL, no ORM. Migrations live in `internal/database/migrations/`, handwritten queries in `internal/database/queries/`, generated code in `internal/database/db/`.

The simplified migration baseline requires a fresh local database. Before starting this branch against an older checkout's data, stop the server and point `DB_PATH` at a new file, then run `make migrate-up`. Sign in again to reconcile retained subscriptions and saves from the PDS; feed content is fetched again. Existing external share and follow records remain untouched.

```sh
make migrate-up                       # apply pending migrations
make migrate-down                     # roll back one
make migrate-status                   # show applied/pending
make migrate-create NAME=add_users    # scaffold a new SQL migration
make sqlc                             # regenerate queries after edits
```

## Build & test

```sh
make build                          # build frontend + Go binary for the host OS → ./main
make build-linux                    # cross-compile a static Linux binary for deploy → ./main-linux-amd64
make build-linux GOARCH=arm64       # same, for ARM VPSes (Hetzner CAX, AWS Graviton, …)
make test                           # go test ./... -v
```

`build` and `build-linux` both run the frontend build first (`bun run build` in `frontend/`) and embed the resulting `frontend/dist/` into the binary via `//go:embed`. The deploy binary is fully self-contained — `scp main-linux-amd64` to the VPS, set env vars, and run.

## Container deployment

The committed `Dockerfile` builds the frontend, embeds it in the Go binary, and runs pending goose migrations before each start. Persist `/data` and set `DB_PATH=/data/morgenblau.db`. A local SMTP mapping uses an unprivileged container port:

```sh
docker build -t morgenblau .
docker run --rm \
  -p 8000:8000 -p 25:2525 \
  -v morgenblau-data:/data \
  --env-file .env \
  morgenblau
```

`fly.toml` is a deployment template. Replace its app name and newsletter domain, then create one volume and keep exactly one Machine because SQLite is local to that volume:

```sh
fly apps create <app-name>
fly volumes create morgenblau_data --region zrh --size 1 --app <app-name>
fly ips allocate-v4 --app <app-name>
fly secrets set --app <app-name> \
  SESSION_COOKIE_KEY='<base64-key>' \
  SESSION_STORE_KEYS='<base64-key>' \
  BLUESKY_OAUTH_PRIVATE_KEY='<base64-key>' \
  BLUESKY_CLIENT_ID='https://<app-host>/oauth-client-metadata.json' \
  BLUESKY_REDIRECT='https://<app-host>/oauth/callback'
```

The dedicated IPv4 is required for raw SMTP on port 25. Point `SMTP_HOSTNAME` at that address and make it the MX target for `NEWSLETTER_DOMAIN`. The Fly TCP service has no TLS handler because the application must negotiate SMTP STARTTLS itself.

Provision a certificate for `SMTP_HOSTNAME` with Certbot and your DNS provider's DNS-01 plugin. Upload it as secrets before accepting mail:

```sh
export FLY_APP=<app-name>
export RENEWED_LINEAGE=/etc/letsencrypt/live/<newsletter-domain>
sh scripts/update-fly-smtp-cert.sh
fly deploy --app <app-name>
fly scale count 1 --app <app-name>
```

Use the same script as Certbot's deploy hook for renewal:

```sh
FLY_APP=<app-name> certbot renew \
  --deploy-hook 'sh /absolute/path/to/morgenblau/scripts/update-fly-smtp-cert.sh'
```

`fly secrets set` restarts the Machine with the renewed certificate. The persistent `/data` volume survives that restart.

The receiver temporarily rejects mail with SMTP 451 before storage fills. The internal high-water limits in `internal/newsletter/ingest.go` reserve 64 MiB for pending receipts and cap stored newsletter data at 256 MiB per owner and 768 MiB globally.

## Git & PRs

[GitHub](https://github.com/hfrdmnk/morgenblau) is the source of truth; [Tangled](https://tangled.org/dominikhofer.me/morgenblau) is a read-only mirror. Open pull requests on GitHub.

Local pushes reach both forges through `origin`. After merging a pull request in GitHub, run `scripts/sync-tangled.sh` to update Tangled's `main` branch.
