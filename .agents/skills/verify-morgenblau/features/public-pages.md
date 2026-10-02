# Public pages

Pages and endpoints any visitor or infrastructure can reach without a session: the about page, the health check, and the OAuth client documents an authorization server fetches.

Help article: none

## Sub-features

- `about` `/about` is a server-rendered page describing Morgenblau.
- `health` `/api/health` answers `200` with `{"status":"up"}`, or still `200` with `{"status":"down","error":...}` when the database ping fails (`healthHandler` in `internal/server/routes.go`).
- `oauth-metadata` `/oauth-client-metadata.json` and `/oauth-jwks.json` serve the OAuth client documents.
- `dev-routes-local-only` `/dev/*` exists only when `APP_ENV=local`; `/dev/login` also needs `DEV_LOGIN_ENABLED=true` and a valid dev account (`LoadDevConfig` in `internal/session/dev.go`).

## How to reach it

- Browser or HTTP: `/about`, `/api/health`, `/oauth-client-metadata.json`, `/oauth-jwks.json`.

## Drive and prove

Preconditions:

- Instance up. No login.

- **About.** `$V api $S GET /about --anon --out about`: `200`, HTML with `<h1>Morgenblau</h1>`. `$V browser $S open /about` and `screenshot --filename=about.png`.
- **Health.** `$V api $S GET /api/health --anon --out health`: `200`, `{"status":"up"}`.
- **OAuth documents.** `$V api $S GET /oauth-client-metadata.json --anon --out client-metadata` and `GET /oauth-jwks.json --out jwks`: `200` JSON each. The JWKS must hold only public key members (no `d`): `jq '[.keys[] | has("d")] | any' $E/jwks.json` is `false`. On an instance the client is in loopback mode and `keys` is usually empty, which makes that check vacuous; record the key count.
- **Dev login availability.** `$V api $S GET /dev/login --anon --out dev-login`: `{"enabled":true}` on a doctor-exit-0 run, `404` otherwise.
- **Negative.** `$V api $S GET /api/unknown --anon`: `401` (the API is gated before routing); `GET /login` while anonymous is `200`.

## Evidence

- `about.json` + `.status`, `about.png`
- `health.json`, `client-metadata.json`, `jwks.json`, `dev-login.json`

## Gotchas

- `$V api` names its output `.json` even for HTML bodies such as `/about`.
- `/dev/*` is served only under `APP_ENV=local`; every instance runs local, so proving the production 404 needs a code-level test, not a drive.
