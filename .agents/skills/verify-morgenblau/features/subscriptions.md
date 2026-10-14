# Subscriptions and Sources

A reader follows RSS or Atom feeds and Standardfeed publications. Adding one writes a subscription record to the reader's PDS first and then mirrors it into the local index; a fetch job loads its entries. The Sources page lists feeds and newsletters, and the header's `Add a source` dialog finds and adds feeds. Renaming, tagging, marking primary, changing the feed URL and removing are API-only for now. Bulk import and export live in [import-export](./import-export.md).

Help article: none

## Sub-features

- `sources-page` `/sources` (`frontend/src/pages/sources.tsx`) lists `Feeds` and `Newsletters` (plus `Stopped newsletters` when any); each list has a `Try again` on load failure. `FeedHealth` shows a red dot after the URL only for unavailable feeds, with an accessible status label; healthy and unchecked feeds have no dot. Generated feeds add a quiet health line ([generated-feeds](./generated-feeds.md)). `FeedList` polls so recovery appears without reloading. No feeds shows `No feeds yet. Import feeds`.
- `source-detail` `/sources/<rkey>` and `/sources/newsletters/<id>` (`frontend/src/pages/source.tsx`, `newsletter-source.tsx`), reached by a source's title on `/sources` or the source name in the reader. The header shows the facts line, tags and the same health as the list: `Posts aren’t fetching right now` with the red dot for an unavailable feed, and the generated-feed note. Its posts open the reader with `Back to source`.
- `add-dialog` `Add a source` (`frontend/src/components/add-source-dialog.tsx`, in the app header on every app page) takes `Website or feed URL`, `Find feeds` lists `Available feeds` with `Select <title>` buttons, `Save` adds the selection and toasts `Source added`. When resolve finds nothing, the dialog offers to generate a feed instead ([generated-feeds](./generated-feeds.md)). The same dialog shows the private `Newsletter email address` with a copy button.
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

- Doctor exit `0`, `$V login $S`, browser signed in. `F` is the run's fixture origin (`fixtures.url` in `up`'s state). No public network is needed except for the Standardfeed step.
- The feed under test is `$F/feeds/valid.xml` (`Example Fixture Feed`, one entry dated a minute before each fetch and one from 2024), found from the site `$F/` alongside `$F/feeds/atom.xml`. Other fixture feeds below prove failure handling; [`tools/feed-fixtures.mjs`](../tools/feed-fixtures.mjs) owns the catalog. A task that names its own public feed may use it instead and record it in `$E/notes.md`.
- An add commits the PDS record before any fetch, so a bad feed URL yields a subscription whose fetch fails, not a refusal.

- **Sources page.** `goto /sources`, `snapshot`: regions `Feeds` and `Newsletters` with the run's feeds (none on a fresh run). `screenshot --filename=sources.png --full-page`.
- **Dialog.** `click "getByRole('button', { name: 'Add a source' })"`: dialog `Add a source` with textbox `Website or feed URL`, a disabled `Find feeds`, and the `Newsletter email address`. `screenshot --filename=add-source-dialog.png`.
- **Find and save.** `fill "getByRole('textbox', { name: 'Website or feed URL' })" "$F/"`, click `Find feeds`: list `Available feeds` with `Select Example Fixture Feed` and `Select Example Fixture Atom`. Click the first (`0 selected` becomes `1 selected`), click `Save`. `inspect`: a `user_subscriptions` row for `$F/feeds/valid.xml`; `/sources` lists it. `screenshot --filename=add-source-saved.png`.
- **Health and recovery.** After `GET /api/jobs/active` is `null`, `GET /api/subscriptions` reports `fetchStatus: "ready"` and a `lastFetchedAt` for the working feed; `/sources` shows no dot after its URL. Add `$F/feeds/unavailable.xml` (always `503`): its first failure reports `fetchStatus: "unavailable"` and `nextFetchAt`, and the page shows a red dot with an accessible automatic-retry label. For recovery, `curl -X POST $F/control/toggle/down`, add `$F/feeds/toggle.xml` and wait for its red dot. Then `curl -X POST $F/control/toggle/up`, move its retry eligibility into the past in the disposable DB only (`sqlite3 $E/run/morgenblau.db "update feeds set next_fetch_at='2000-01-01T00:00:00Z' where feed_url='$F/feeds/toggle.xml'"`) and call `POST /api/digest/refresh`. Wait for the job to finish and the page to poll: the dot disappears without navigation or reload. Capture both states and check a narrow viewport for overflow.
- **Odd feeds.** Add `$F/feeds/atom.xml`, `$F/feeds/stale.xml` and `$F/feeds/latin1.xml`: each is `ready`, and their entries include `Atom fixture entry published just now`, `Stale fixture entry from 2019` and `Grüße aus Zürich, café crème` (served as ISO-8859-1, stored as UTF-8). Add `$F/feeds/broken.xml` (malformed XML) and `$F/feeds/huge.xml` (over the fetcher's 5 MiB cap): each is `unavailable` with no entries, and `server.log` shows `parse feed` and `response body exceeds cap`. `GET "/api/digest?timezone=<the browser's zone>"` holds the fresh valid, Atom and ISO-8859-1 entries and never the 2019 one.
- **Fixture boundary.** Only the fixture origin is reachable. Add the decoy (`fixtures.decoy_url` + `/feed.xml`, a valid feed on another loopback port) and `$F/feeds/redirect-decoy.xml` (a `302` to the decoy): both are `unavailable`, `feed_entries` never holds `Decoy entry`, and `server.log` shows `safehttp: blocked address` for the decoy port on both fetches. `POST /api/subscriptions/resolve` for the decoy URL answers `502` `upstream_error` (`Couldn't reach that URL`), and `server.log` shows the same blocked address.
- **Resolve.** `$V api $S POST /api/subscriptions/resolve --data '{"url":"'$F'/feeds/valid.xml"}' --out resolve`: `200`, the candidate, and the new subscription under `existingSubscriptions`.
- **Read.** `GET /api/subscriptions/<rkey> --out sub`, `GET /api/subscriptions/<rkey>/entries --out entries`: `Fixture entry published just now` and `Fixture entry from 2024` once `GET /api/jobs/active` is `null`.
- **Add again.** `POST /api/subscriptions --data '{"feedUrl":"'$F'/feeds/valid.xml"}' --out add-again`: `200`, the same `rkey`, no `jobId`.
- **Other kind for a followed site.** Needs network, since no fixture can publish a Standardfeed publication; without it this step is `blocked`. While the run follows the user's approved standing feed `https://dominikhofer.me/rss`, `POST /api/subscriptions/resolve --data '{"url":"https://dominikhofer.me"}' --out resolve-site` shows its Standardfeed candidate with `subscribedVia` (the dialog shows it disabled, `Already followed via another feed.`). Post that candidate as the dialog would, `--data '{"publication":"<its publication>","siteUrl":"<its siteUrl>"}' --out add-publication-conflict`: `409` with the site message and unchanged `inspect` counts.
- **Edit.** `PATCH /api/subscriptions/<rkey> --data '{"title":"Example Feed","tags":["example"]}' --out patch`: `200`; `GET /api/subscriptions/tags` includes `example`. `PATCH` with `{"feedUrl":"$F/feeds/valid.xml?verify=1"}` `--out patch-feedurl`: `200` with a `jobId`.
- **Negative.** After Edit moved the subscription to `$F/feeds/valid.xml?verify=1`, add `$F/feeds/valid.xml` again as a second subscription (`--out add-second`: a new `rkey`), then `PATCH` the second one's `feedUrl` to `$F/feeds/valid.xml?verify=1` `--out patch-conflict`: `409` `already subscribed to that feed`. `PATCH /api/subscriptions/unknownrkey --data '{"title":"x"}'`: `404`. `POST /api/subscriptions --data '{}' --out add-invalid`: `400` with `errors["feedUrl"]`. `POST /api/subscriptions --data '[{"feedUrl":"'$F'/feeds/valid.xml"}]' --out add-list`: `400` `invalid json`, and no new row or PDS record. `GET /api/subscriptions --anon`: `401`.
- **Remove.** `DELETE /api/subscriptions/<rkey>` for both valid-feed subscriptions: `204` each; the rows are gone from `inspect` and from `/sources` after `reload`.

## Evidence

- `notes.md` (the fixture origin, or the task's feed URL), `sources.png`, `add-source-dialog.png`, `add-source-saved.png`, the health states
- `subscriptions-fixtures.json` (`GET /api/subscriptions` after the odd-feed and boundary adds), `boundary.txt` (the decoy's `server.log` lines and the empty `Decoy entry` query)
- `resolve.json`, `sub.json`, `entries.json`, `add-again.json`, `resolve-site.json`, `add-publication-conflict.json`, `patch.json`, `patch-feedurl.json`, `add-second.json`, `patch-conflict.json`, `add-invalid.json`, `add-list.json`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- These are real writes to the run's own PDS, deleted with it by `down`.
- Fixture feeds are reachable only because `up` passes the fixture origin as `FEED_FIXTURE_ORIGIN`, which `loadLocalNetwork` (`internal/server/`) honors only beside a run's `PLC_URL` under `APP_ENV=local`. Other loopback ports, private addresses and redirects off the origin stay blocked; production ignores the variable.
- Favicon discovery keeps the default guarded client, so fixture feeds show the fallback icon and `server.log` warns `favicon discovery failed`. Not a failure.
- An API add without a `title` lists the feed under its site URL; the dialog sends the feed's title.
- A fresh fixture entry is dated a minute before the fetch, so in the first minute after midnight it lands on the previous day.
- Standardfeed adds resolve the publication's identity over the network; a failure there is `blocked`, not `fail`.
- A session without the Standardfeed scope gets `403` with `code: "reauth_required"` on Standardfeed writes; the dev login's password session is not scoped this way.
- The dialog prefixes a bare host with `https://` before resolving (`frontend/src/lib/add-source.ts`).
