# Public pages

Pages and endpoints any visitor or infrastructure can reach without a session: the about page, the health check, and the OAuth client documents an authorization server fetches.

Help article: none

## Sub-features

- `about` `/about` is a server-rendered page describing Morgenblau.
- `health` `/api/health` answers `200` with `{"status":"up"}`, or still `200` with `{"status":"down","error":...}` when the database ping fails (`healthHandler` in `internal/server/routes.go`).
- `web-feed` `GET /web/<id>` serves a generated feed as RSS without a session, so other feed readers can follow it too; an unknown id is `404` ([generated-feeds](./generated-feeds.md)).
- `oauth-metadata` `/oauth-client-metadata.json` and `/oauth-jwks.json` serve the OAuth client documents.
- `dev-routes-local-only` `/__dev/*` exists only when `APP_ENV=local`; `/__dev/login` also needs `DEV_LOGIN_ENABLED=true` and a valid account (`LoadDevConfig` in `internal/session/dev.go`), which `up` creates on the run's PDS.

## How to reach it

- Browser or HTTP: `/about`, `/api/health`, `/web/<id>`, `/oauth-client-metadata.json`, `/oauth-jwks.json`.

## Drive and prove

Preconditions:

- Instance up. No login.

- **About.** `$V api $S GET /about --anon --out about`: `200`, HTML with `<h1>Morgenblau</h1>`. `$V browser $S open /about` and `screenshot --filename=about.png`.
- **Health.** `$V api $S GET /api/health --anon --out health`: `200`, `{"status":"up"}`.
- **OAuth documents.** `$V api $S GET /oauth-client-metadata.json --anon --out client-metadata` and `GET /oauth-jwks.json --out jwks`: `200` JSON each. The JWKS must hold only public key members (no `d`): `jq '[.keys[] | has("d")] | any' $E/jwks.json` is `false`. On an instance the client is in loopback mode and `keys` is usually empty, which makes that check vacuous; record the key count.
- **Generated feed.** With a `web_sources` row arranged as in [generated-feeds](./generated-feeds.md), `$V api $S GET /web/<id> --anon --out web-feed-anon`: `200`, `<rss version="2.0">` with one `<item>` per stored post. `GET /web/unknown --anon`: `404`.
- **Dev login availability.** `$V api $S GET /__dev/login --anon --out dev-login`: `{"enabled":true}` on a doctor-exit-0 run, `404` otherwise. The old `GET /dev/login --anon` is an ordinary gated path: a `302` to `/login`, never the dev login.
- **Negative.** `$V api $S GET /api/unknown --anon`: `401` (the API is gated before routing); `GET /login` while anonymous is `200`.

## Evidence

- `about.json` + `.status`, `about.png`
- `web-feed-anon.json`
- `health.json`, `client-metadata.json`, `jwks.json`, `dev-login.json`

## Gotchas

- `$V api` names its output `.json` even for HTML bodies such as `/about`.
- `/__dev/*` is served only under `APP_ENV=local`; every instance runs local, so the production 404 is proven by `TestDevRoutesAreNotServedOutsideLocalEnvironment` and `TestDevLoginRouteRequiresLocalOptIn` in `internal/server/routes_test.go`, not a drive. The `__dev` prefix is a naming convention; `RegisterRoutes` is the gate.
