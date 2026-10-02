# Subscriptions and Sources

A reader follows RSS or Atom feeds and Standardfeed publications. Adding one writes a subscription record to the reader's PDS first and then mirrors it into the local index; a fetch job loads its entries. The Sources page lists feeds and newsletters, and the header's `Add a source` dialog finds and adds feeds. Renaming, tagging, marking primary, changing the feed URL and removing are API-only for now. Bulk import and export live in [import-export](./import-export.md).

Help article: none

## Sub-features

- `sources-page` `/sources` (`frontend/src/pages/sources.tsx`) lists `Feeds` (title, site or feed URL, `Primary`, a muted note for failing feeds) and `Newsletters` (plus `Stopped newsletters` when any); each list has a `Try again` on load failure. No feeds shows `No feeds yet. Import feeds`.
- `add-dialog` `Add a source` (`frontend/src/components/add-source-dialog.tsx`, in the app header on every app page) takes `Website or feed URL`, `Find feeds` lists `Available feeds` with `Select <title>` buttons, `Save` adds the selection and toasts `Source added`. The same dialog shows the private `Newsletter email address` with a copy button.
- `resolve` `POST /api/subscriptions/resolve` with `{url}` returns `{candidates, existingSubscriptions}`.
- `add` `POST /api/subscriptions` with `{"subscriptions":[{"feedUrl":"..."}]}` (RSS) or `{"subscriptions":[{"publication":"at://..."}]}` (Standardfeed); returns `{records, jobIds}`. Adding an existing feed returns its record and no job. A missing URL answers `400` with field errors.
- `list` `GET /api/subscriptions`, `GET /api/subscriptions/{rkey}`, `GET /api/subscriptions/{rkey}/entries`, `GET /api/subscriptions/tags`.
- `edit` `PATCH /api/subscriptions/{rkey}` with any of `title`, `primary`, `tags`, `feedUrl`; a `feedUrl` change returns a `jobId`, and one that matches another subscription answers `409` `already subscribed to that feed`.
- `remove` `DELETE /api/subscriptions/{rkey}`.

## How to reach it

- Browser: `/sources`, or the mode selector (`<mode>: choose mode`, then `Sources`). `Add a source` in the header of `/`, `/sources`, `/library` and `/settings/*`.
- API routes above (`internal/server/routes.go`).

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S` (records the PDS baseline), browser signed in. Network access to the dev account's PDS and to the feed.
- A public feed URL for this run: the task's, else the user's own `https://dominikhofer.me/rss`, which they approved as the standing test feed. Record the one used in `$E/notes.md`. A loopback fixture feed is refused by the SSRF guard, and an add writes the PDS record before any fetch, so never probe with a bad feed URL.

- **Sources page.** `goto /sources`, `snapshot`: regions `Feeds` and `Newsletters` with the dev account's feeds. `screenshot --filename=sources.png --full-page`.
- **Dialog.** `click "getByRole('button', { name: 'Add a source' })"`: dialog `Add a source` with textbox `Website or feed URL`, a disabled `Find feeds`, and the `Newsletter email address`. `screenshot --filename=add-source-dialog.png`.
- **Find and save.** `fill "getByRole('textbox', { name: 'Website or feed URL' })" "<feed url>"`, click `Find feeds`, wait a few seconds: list `Available feeds` with `Select <title>`. Click it (`0 selected` becomes `1 selected`), click `Save`. `inspect`: a `user_subscriptions` row for the URL; `/sources` lists it. `screenshot --filename=add-source-saved.png`.
- **Resolve.** `$V api $S POST /api/subscriptions/resolve --data '{"url":"<feed url>"}' --out resolve`: `200`, the candidate, and the new subscription under `existingSubscriptions`.
- **Read.** `GET /api/subscriptions/<rkey> --out sub`, `GET /api/subscriptions/<rkey>/entries --out entries`: fetched entries once `GET /api/jobs/active` is `null`.
- **Add again.** `POST /api/subscriptions --data '{"subscriptions":[{"feedUrl":"<feed url>"}]}' --out add-again`: `200`, one record, empty `jobIds`.
- **Edit.** `PATCH /api/subscriptions/<rkey> --data '{"title":"Example Feed","tags":["example"]}' --out patch`: `200`; `GET /api/subscriptions/tags` includes `example`. `PATCH` with `{"feedUrl":"<feed url>?verify=1"}` `--out patch-feedurl`: `200` with a `jobId`.
- **Remove.** `DELETE /api/subscriptions/<rkey>`: `204`; the row is gone from `inspect` and from `/sources` after `reload`, and `down` has nothing left to delete for it.
- **Negative.** `PATCH` the new subscription's `feedUrl` to another subscription's feed URL: `409` (run this before Remove). `PATCH /api/subscriptions/unknownrkey --data '{"title":"x"}'`: `404`. `POST /api/subscriptions --data '{"subscriptions":[{}]}' --out add-invalid`: `400` with `errors["subscriptions.0.feedUrl"]`. `GET /api/subscriptions --anon`: `401`.

## Evidence

- `notes.md` (the feed URL used), `sources.png`, `add-source-dialog.png`, `add-source-saved.png`
- `resolve.json`, `sub.json`, `entries.json`, `add-again.json`, `patch.json`, `patch-feedurl.json`, `add-invalid.json`
- `inspect-before.json`, `inspect-after.json`, `cleanup.log` when `down` deleted anything

## Gotchas

- These are real PDS writes on the dev account. `down` deletes subscriptions whose rkey was not in the login baseline; run `$V login $S` before any add, and `down` while the instance is still running.
- Standardfeed adds resolve the publication's identity over the network; a failure there is `blocked`, not `fail`.
- A session without the Standardfeed scope gets `403` with `code: "reauth_required"` on Standardfeed writes; the dev login's password session is not scoped this way.
- The dialog prefixes a bare host with `https://` before resolving (`frontend/src/lib/add-source.ts`).
