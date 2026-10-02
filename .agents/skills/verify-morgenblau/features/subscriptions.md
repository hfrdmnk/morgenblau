# Subscriptions and Sources

A reader follows RSS or Atom feeds and Standardfeed publications. Adding one writes a subscription record to the reader's PDS first and then mirrors it into the local index; a fetch job loads its entries. The Sources page lists feeds and newsletters, and the header's `Add a source` dialog finds and adds feeds. Renaming, tagging, marking primary, changing the feed URL and removing are API-only for now. Bulk import and export live in [import-export](./import-export.md).

Help article: none

## Sub-features

- `sources-page` `/sources` (`frontend/src/pages/sources.tsx`) lists `Feeds` (title, site or feed URL, `Primary`, a muted note for failing feeds) and `Newsletters` (plus `Stopped newsletters` when any); each list has a `Try again` on load failure. No feeds shows `No feeds yet. Import feeds`.
- `add-dialog` `Add a source` (`frontend/src/components/add-source-dialog.tsx`, in the app header on every app page) takes `Website or feed URL`, `Find feeds` lists `Available feeds` with `Select <title>` buttons, `Save` adds the selection and toasts `Source added`. The same dialog shows the private `Newsletter email address` with a copy button.
- `resolve` `POST /api/subscriptions/resolve` with `{url}` returns `{candidates, existingSubscriptions}`.
- `add` `POST /api/subscriptions` with one source per request, `{"feedUrl":"..."}` (RSS) or `{"publication":"at://..."}` (Standardfeed); returns the record plus its `jobId`. Adding an existing feed returns its record without a `jobId`. A missing URL answers `400` with field errors; a list body is `invalid json`. A source for a site the reader already follows under the other kind answers `409` `Pick either the RSS feed or the ATProto publication for a site, not both`, before any PDS write.
- `list` `GET /api/subscriptions`, `GET /api/subscriptions/{rkey}`, `GET /api/subscriptions/{rkey}/entries`, `GET /api/subscriptions/tags`.
- `edit` `PATCH /api/subscriptions/{rkey}` with any of `title`, `primary`, `tags`, `feedUrl`; a `feedUrl` change returns a `jobId`, and one that matches another subscription answers `409` `already subscribed to that feed`. One that moves onto a site followed as a Standardfeed publication answers the site `409` above.
- `remove` `DELETE /api/subscriptions/{rkey}`.

## How to reach it

- Browser: `/sources`, or the mode selector (`<mode>: choose mode`, then `Sources`). `Add a source` in the header of `/`, `/sources`, `/library` and `/settings/*`.
- API routes above (`internal/server/routes.go`).

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, browser signed in. Network access to the feed.
- A public feed URL for this run: the task's, else the user's own `https://dominikhofer.me/rss`, which they approved as the standing test feed. Record the one used in `$E/notes.md`. A loopback fixture feed is refused by the SSRF guard. An add commits the PDS record before any fetch, so a bad feed URL yields a subscription whose fetch fails, not a refusal; it proves nothing.

- **Sources page.** `goto /sources`, `snapshot`: regions `Feeds` and `Newsletters` with the run's feeds (none on a fresh run). `screenshot --filename=sources.png --full-page`.
- **Dialog.** `click "getByRole('button', { name: 'Add a source' })"`: dialog `Add a source` with textbox `Website or feed URL`, a disabled `Find feeds`, and the `Newsletter email address`. `screenshot --filename=add-source-dialog.png`.
- **Find and save.** `fill "getByRole('textbox', { name: 'Website or feed URL' })" "<feed url>"`, click `Find feeds`, wait a few seconds: list `Available feeds` with `Select <title>`. Click it (`0 selected` becomes `1 selected`), click `Save`. `inspect`: a `user_subscriptions` row for the URL; `/sources` lists it. `screenshot --filename=add-source-saved.png`.
- **Resolve.** `$V api $S POST /api/subscriptions/resolve --data '{"url":"<feed url>"}' --out resolve`: `200`, the candidate, and the new subscription under `existingSubscriptions`.
- **Read.** `GET /api/subscriptions/<rkey> --out sub`, `GET /api/subscriptions/<rkey>/entries --out entries`: fetched entries once `GET /api/jobs/active` is `null`.
- **Add again.** `POST /api/subscriptions --data '{"feedUrl":"<feed url>"}' --out add-again`: `200`, the same `rkey`, no `jobId`.
- **Other kind for a followed site.** While the run follows the standing site's RSS feed (`https://dominikhofer.me/rss`; when the run's feed is another, add the standing one first), `POST /api/subscriptions/resolve --data '{"url":"https://dominikhofer.me"}' --out resolve-site` shows its Standardfeed candidate with `subscribedVia` (the dialog shows it disabled, `Already followed via another feed.`). Post that candidate as the dialog would, `--data '{"publication":"<its publication>","siteUrl":"<its siteUrl>"}' --out add-publication-conflict`: `409` with the site message and unchanged `inspect` counts.
- **Edit.** `PATCH /api/subscriptions/<rkey> --data '{"title":"Example Feed","tags":["example"]}' --out patch`: `200`; `GET /api/subscriptions/tags` includes `example`. `PATCH` with `{"feedUrl":"<feed url>?verify=1"}` `--out patch-feedurl`: `200` with a `jobId`.
- **Negative.** After Edit moved the subscription to `<feed url>?verify=1`, add `<feed url>` again as a second subscription (`POST /api/subscriptions --data '{"feedUrl":"<feed url>"}' --out add-second`: a new `rkey`), then `PATCH` the second one's `feedUrl` to `<feed url>?verify=1` `--out patch-conflict`: `409` `already subscribed to that feed`. `PATCH /api/subscriptions/unknownrkey --data '{"title":"x"}'`: `404`. `POST /api/subscriptions --data '{}' --out add-invalid`: `400` with `errors["feedUrl"]`. `POST /api/subscriptions --data '[{"feedUrl":"<feed url>"}]' --out add-list`: `400` `invalid json`, and no new row or PDS record. `GET /api/subscriptions --anon`: `401`.
- **Remove.** `DELETE /api/subscriptions/<rkey>` for both subscriptions: `204` each; the rows are gone from `inspect` and from `/sources` after `reload`.

## Evidence

- `notes.md` (the feed URL used), `sources.png`, `add-source-dialog.png`, `add-source-saved.png`
- `resolve.json`, `sub.json`, `entries.json`, `add-again.json`, `resolve-site.json`, `add-publication-conflict.json`, `patch.json`, `patch-feedurl.json`, `add-second.json`, `patch-conflict.json`, `add-invalid.json`, `add-list.json`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- These are real writes to the run's own PDS, deleted with it by `down`.
- Standardfeed adds resolve the publication's identity over the network; a failure there is `blocked`, not `fail`.
- A session without the Standardfeed scope gets `403` with `code: "reauth_required"` on Standardfeed writes; the dev login's password session is not scoped this way.
- The dialog prefixes a bare host with `https://` before resolving (`frontend/src/lib/add-source.ts`).
