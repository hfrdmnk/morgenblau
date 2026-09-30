# Sync jobs

Background work that keeps the local reading index in step with the PDS and the feeds. Signing in starts a sync that reconciles subscriptions and saves from the PDS and fetches their feeds. A reader can refresh the digest by hand. A global refresher re-fetches every feed on an interval. Newsletter receipts are processed by their own loop (see [newsletters](./newsletters.md)).

Help article: none

## Sub-features

- `login-sync` signing in dispatches a sync; `GET /api/jobs/active` lists it until done.
- `manual-refresh` `POST /api/digest/refresh` returns `{jobId}`; `GET /api/jobs/{id}` reports its state.
- `latest` `GET /api/jobs/latest` returns the active sync or the last unresolved failure.
- `digest-flag` `GET /api/digest` reports `hasActiveJob` true while a sync runs.
- `global-refresh` with `FETCH_INTERVAL_MINUTES` above 0 the server re-fetches all feeds on that interval.

## How to reach it

- Sign in (`$V login $S` or the `Log me in` button).
- API: `POST /api/digest/refresh`, `GET /api/jobs/active`, `GET /api/jobs/latest`, `GET /api/jobs/{id}`.
- Scheduled: start the instance with `$V up $S --fetch-minutes 1`.

## Drive and prove

Preconditions:

- Doctor exit `0`. `$V inspect $S --out inspect-before` before login.

- **Login sync.** `$V login $S` waits for the jobs to finish. `$V inspect $S --out inspect-after-login`: `user_subscriptions` and `user_saves` now mirror the dev account's PDS records (compare their counts with `GET /api/subscriptions` and `GET /api/saves`).
- **Manual refresh.** `$V api $S POST /api/digest/refresh --out refresh`: `{jobId}`. `GET /api/jobs/<jobId> --out job` until its status is terminal; `GET /api/jobs/latest --out latest` agrees.
- **Global refresh.** On a `--fetch-minutes 1` instance, note `feeds.last_fetched_at` via `sqlite3 -readonly $E/run/morgenblau.db "select feed_url, last_fetched_at from feeds"`, wait past a minute, read again: it advanced. `server.log` shows `global feed fetch enabled`.
- **Negative.** `GET /api/jobs/unknownid`: `404`. `POST /api/digest/refresh --anon`: `401`.

## Evidence

- `inspect-before.json`, `inspect-after-login.json`
- `refresh.json`, `job.json`, `latest.json`
- `server.log` (kept by `down`)

## Gotchas

- A dev account with no subscriptions syncs to empty tables; that is a valid pass for reconciliation but proves no feed fetch.
- Feed fetches reach the public internet. Individual feed failures do not fail the sync (LAWS.md, law 2); read the job state, not the fetch log.
- The global refresher only walks feeds in the run's own DB, so turning it on touches nothing outside the run.
