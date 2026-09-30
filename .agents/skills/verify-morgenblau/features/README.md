# Morgenblau feature map

What Morgenblau does, how a user reaches each feature, and what proves it works. Each file is a recipe for `verify-morgenblau`; `V` is `.agents/skills/verify-morgenblau/bin/verify` and `S` is the run's slug.

## Baseline

- `$V doctor` exits `0` (or `3`, which blocks every signed-in step).
- A fresh instance for this run: `$V up $S`. Its `base_url` is `http://127.0.0.1:<port>`; a `/path` passed to `$V browser` expands to it.
- Signed-in recipes start with `$V login $S`. The signed-in user is the configured dev account; its DID and handle come from `$V api $S GET /api/profiles/me`.
- Most of the UI is still placeholders (see Unmapped). The API is the entry point the frontend will call, so API recipes drive `/api/*` exactly as `frontend/src/lib/api.ts` does.

## Features

UI and public:

- [Sign in and out](./sign-in.md): `/login`, the dev `Log me in` button, anonymous redirects, the account menu's `Log out`.
- [Public pages](./public-pages.md): `/about`, `/api/health`, OAuth client metadata, `/dev/login` availability.
- [Daily digest](./digest.md): `/` and `/?date=`, date navigation, digest API, entry detail API.

API and background:

- [Subscriptions](./subscriptions.md): resolve, add, edit, remove RSS and Standardfeed subscriptions (PDS writes).
- [URL saves](./saves.md): save and unsave a web URL (PDS writes), the saves list.
- [Sync jobs](./sync-jobs.md): login sync, manual digest refresh, job status, the global feed refresher.

Email:

- [Private newsletters](./newsletters.md): the private address, SMTP delivery, sources, remote-image consent, moving, saving, digest inclusion.

Isolation:

- [Reader isolation](./tenant-isolation.md): a second reader's newsletter data is 404 for the dev account and 401 for anonymous callers.

## Unmapped

- Library page: `frontend/src/pages/library.tsx` (placeholder).
- Sources page and source detail: `frontend/src/pages/sources.tsx`, `frontend/src/pages/source.tsx`, `frontend/src/pages/newsletter-source.tsx` (placeholders).
- Settings page: `frontend/src/pages/settings.tsx` (placeholder).
- Entry reader page: `frontend/src/pages/entry.tsx` (placeholder; the entry API is in [digest](./digest.md)).
- Digest header `Add a source` button and `Digest` switcher: `frontend/src/pages/digest.tsx` (rendered without handlers).
- OAuth handle sign-in end to end: `internal/oauth/handler/flow.go` (needs a real account's consent screen in the user's browser).
- Favicon proxy: `GET /api/favicon` in `internal/api/favicon.go`.
- Dev styleguide: `/dev/styleguide` from `frontend/src/dev/`.
