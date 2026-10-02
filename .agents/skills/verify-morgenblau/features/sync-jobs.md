# Sync jobs

Background work that keeps the local reading index in step with the PDS and the feeds. Signing in starts a sync that reconciles subscriptions and saves from the PDS and fetches their feeds. A reader can refresh the digest by hand. A global refresher re-fetches every feed on an interval. Newsletter receipts are processed by their own loop (see [newsletters](./newsletters.md)).

Help article: none

## Sub-features

- `login-sync` signing in dispatches a sync; `GET /api/jobs/active` returns it as one object until done, then `null`.
- `manual-refresh` `POST /api/digest/refresh` returns `{jobId}`; `GET /api/jobs/{id}` reports its state, and `404` for an unknown id or another reader's job.
- `latest` `GET /api/jobs/latest` returns the active sync, else the last unresolved failure, else the most recent sync even when done (`LatestSyncForUser` in `internal/jobs/jobs.go`).
- `digest-flag` `GET /api/digest` reports `hasActiveJob` true while a sync runs.
- `global-refresh` with `FETCH_INTERVAL_MINUTES` above 0 the server re-fetches all feeds on that interval. It is not a job: `/api/jobs/active` stays `null`.

## How to reach it

- Sign in (`$V login $S` or the `Log me in` button).
- API: `POST /api/digest/refresh`, `GET /api/jobs/active`, `GET /api/jobs/latest`, `GET /api/jobs/{id}`.
- Scheduled: start the instance with `$V up $S --fetch-minutes 1`.

## Drive and prove

Preconditions:

- Doctor exit `0`. `$V inspect $S --out inspect-before` before login.

- **Login sync.** `$V login $S` waits for the jobs to finish. `$V inspect $S --out inspect-after-login`: `user_subscriptions` and `user_saves` now mirror the run account's PDS records (none on a fresh run) (compare their counts with `GET /api/subscriptions` and `GET /api/saves`).
- **Latest after login.** `GET /api/jobs/latest --out latest-after-login`: the login `sync_user` job, status `done`.
- **Manual refresh.** `$V api $S POST /api/digest/refresh --out refresh`: `{jobId}`. Right after, `GET /api/jobs/active --out active-running` is that job (`running`) and `GET /api/digest` has `hasActiveJob` true. `GET /api/jobs/<jobId> --out job` until its status is `done` or `failed`; `GET /api/jobs/latest --out latest` agrees.
- **Global refresh.** On a `--fetch-minutes 1` instance, save `sqlite3 -readonly $E/run/morgenblau.db "select max(last_fetched_at) from feeds"` to `fetched-1.txt`, wait past a minute, save `fetched-2.txt`: it advanced while `/api/jobs/active` stayed `null`. `server.log` shows `global feed fetch enabled interval=1m0s`.
- **Negative.** `GET /api/jobs/unknownid`: `404`. `POST /api/digest/refresh --anon`: `401`.

## Evidence

- `inspect-before.json`, `inspect-after-login.json`
- `latest-after-login.json`, `refresh.json`, `active-running.json`, `job.json`, `latest.json`, `fetched-1.txt`, `fetched-2.txt`
- `server.log` (kept by `down`)

## Gotchas

- A fresh run's account has no subscriptions, so the login sync reconciles empty tables and proves no feed fetch. Add a feed (see [subscriptions](./subscriptions.md)) before the manual refresh when the fetch matters.
- Feed fetches reach the public internet. Individual feed failures do not fail the sync (LAWS.md, law 2); read the job state, not the fetch log.
- The global refresher only walks feeds in the run's own DB, so turning it on touches nothing outside the run.
