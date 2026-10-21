# Morgenblau feature map

What Morgenblau does, how a user reaches each feature, and what proves it works. Each file is a recipe for `verify-morgenblau`; `V` is `.agents/skills/verify-morgenblau/bin/verify` and `S` is the run's slug.

## Baseline

- `$V doctor` exits `0` (or `3`, which blocks every signed-in step).
- A fresh instance for this run: `$V up $S`. Its `base_url` is `http://127.0.0.1:<port>`; a `/path` passed to `$V browser` expands to it.
- Feed fixtures: on a doctor-exit-0 run, `up` also starts a fixture feed server ([`tools/feed-fixtures.mjs`](../tools/feed-fixtures.mjs)) whose origin, `fixtures.url` in the state, is the only loopback feed source the instance may fetch. Recipes use it instead of public feeds, so their feed steps need no network.
- Signed-in recipes start with `$V login $S`. The signed-in user is the run's own `.test` account, which starts with no subscriptions or saves; `$V login $S` prints its DID and handle.
- UI recipes drive the React app through `$V browser`. API recipes drive `/api/*` exactly as `frontend/src/lib/api.ts` does, for surfaces without UI and for exact status and state checks.

## Features

UI and public:

- [Sign in and out](./sign-in.md): `/login`, OAuth sign-in with the run's handle, the dev `Log me in` button, anonymous redirects, the account menu and `Log out`.
- [Public pages](./public-pages.md): `/about`, `/api/health`, OAuth client metadata, `/__dev/login` availability.
- [Daily digest](./digest.md): `/` and `/?date=`, the header date navigation, digest API, entry API.
- [Entry reader](./entry-reader.md): `/entry/<slug>`, `Load full article` (extract), newsletter inline assets and `Load images`, `Back to digest`.
- [Subscriptions and Sources](./subscriptions.md): `/sources`, the `Add a source` dialog, the mode selector, resolve, add, edit, remove (PDS writes).
- [Generated feeds](./generated-feeds.md): `Generate a feed` for a page without one, the preview, pointing at a post, blocked sites, the hourly limit, generated-feed health on `/sources`, the public `/feed/<id>` feed and its `/web/<id>` alias, section pages and their aliases, Livewire lazy lists, browser-built pages.
- [Import and export](./import-export.md): `/settings/import` (OPML, Skyreader, Glean), `/settings/export`, the import and export API (PDS writes).

API and background:

- [URL saves](./saves.md): save and unsave a web URL (PDS writes), the saves list.
- [Sync jobs](./sync-jobs.md): login sync, manual digest refresh, job status, the global feed refresher.

Email:

- [Private newsletters](./newsletters.md): the private address (API, `Add a source`, `/settings/general`), SMTP delivery, sources, stop and enable, remote-image consent, moving, saving.

Isolation:

- [Reader isolation](./tenant-isolation.md): a second reader's newsletter data is 404 for the run's account and 401 for anonymous callers.
- [Admin usage](./admin.md): `/admin` and `/api/admin/usage` for `ADMIN_DIDS` (`up --admin`), 404 for every other reader.

## Unmapped

- Library page: `frontend/src/pages/library.tsx` (placeholder).
- OAuth with the production confidential client: only the loopback public client runs on an instance (see [sign-in](./sign-in.md) gotchas).
- Favicon proxy: `GET /api/favicon` in `internal/api/favicon.go`.
- Dev styleguide: `/__dev/styleguide` from `frontend/src/dev/` (development-only).
