# Admin usage

Operators listed in `ADMIN_DIDS` open `/admin`: an `Admin only` banner over the usual app chrome, totals for people and for regular, generated and newsletter sources, then one row per account with its counts and last session time. Everyone else gets a 404 on `/admin` and `/api/admin/*`, the same answer as a route that does not exist. The auth gate in `internal/middleware/auth/` owns admission; `SPEC.md` (Identity and Sessions) owns the rule.

Help article: none

## Sub-features

- `usage` `GET /api/admin/usage` returns `{totals, people}`; a subscription on this server's own `/feed/<id>` (or the legacy `/web/<id>`) counts as generated, any other as regular, active newsletter sources as newsletters.
- `menu` the account menu lists `Admin` only when `GET /api/profiles/me` says `isAdmin`.
- `hidden` a signed-in reader outside `ADMIN_DIDS` gets 404 for the page and the API; anonymous callers get 401 or the `/login` redirect.

## Drive and prove

Preconditions: two instances, `$V up <slug> --admin` (the run account is in `ADMIN_DIDS`) and a plain `$V up <slug2>`. On the admin run add `$F/feeds/valid.xml` through `POST /api/subscriptions`, give the account an address (`POST /api/newsletters/address`) and `$V mail` it, and add `$V foreign` with mail of its own. A generated subscription must go through the PDS like any other: insert a `web_sources` row (see [generated feeds](./generated-feeds.md)), then `POST /api/subscriptions` with `<base_url>/feed/<id>`. A row written only to SQLite is removed by the next sign-in's reconciliation.

- **Usage.** `$V oauth <slug>` (dev sessions live in memory, so only an OAuth session has a last-seen time), then `GET /api/admin/usage --out api-usage-admin`: each account's counts match what was added. In the browser, `/admin` at 1280×900 and 390×844 with no horizontal overflow (`admin-desktop.png`, `admin-mobile.png`); the account menu shows `Admin` (`admin-account-menu.png`).
- **Negative.** On the plain run after `$V login`, `GET /api/admin/usage` and `GET /admin` answer `404`, `GET /api/profiles/me` has `isAdmin: false`, and the account menu has no `Admin`. Anonymous `GET /api/admin/usage` answers `401`, `/admin` redirects to `/login`.
