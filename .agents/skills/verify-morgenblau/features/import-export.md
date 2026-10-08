# Import and export sources

Readers bring feeds in from an OPML file or from another ATProto reader's records (Skyreader, Glean), and take them out as OPML. Import writes one subscription record per new feed to the PDS, folders become tags, and feeds already followed only gain tags. Export reads the PDS subscriptions and returns OPML with tags as folders. Newsletters and Standardfeed subscriptions are neither imported nor exported.

Help article: none

## Sub-features

- `import-page` `/settings/import` (`frontend/src/pages/import-sources.tsx`): preparing opens a confirmation dialog; successful imports keep a `Subscriptions saved` receipt open with counts. Subscription failures show `Import paused` with `Retry remaining sources`; feed availability is separate. `ImportCollection` polls health for confirmed sources and lists only unavailable feeds in a `Posts not fetching for <n> sources` accordion. `View sources` is a ghost button that closes the receipt and navigates to Sources.
- `prepare` `POST /api/subscriptions/import/prepare` with `{provider:"opml", opml}` parses the file; `skyreader` and `glean` list that app's records on the reader's PDS. Returns `{sources, warnings}`.
- `import` `POST /api/subscriptions/import` with `{sources}` (1 to 5 per call; the page batches by 5, `frontend/src/lib/source-import.ts`) returns `{added, updated, unchanged, failures}`.
- `export-page` `/settings/export`: `Download OPML` saves `morgenblau.opml` and toasts `Your OPML download is ready.`
- `export` `POST /api/subscriptions/export` returns `{opml}`.

## How to reach it

- Browser: the account menu's `Account` navigation (`Import`, `Export`), or the `Settings` navigation on any `/settings/*` page.
- API routes above (`internal/api/subscriptions_import.go`).

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, browser signed in.
- An OPML fixture in the evidence dir, so the guarded `upload` accepts it: write `$E/sources.opml` with one folder `Example Folder` holding two `type="rss"` outlines whose `xmlUrl` are `https://example.com/verify-one.xml` and `https://example.com/verify-two.xml`. Their fetches fail; a feed failure never fails the sync.

- **Prepare.** `$V api $S POST /api/subscriptions/import/prepare --data "$(jq -n --rawfile o $E/sources.opml '{provider:"opml", opml:$o}')" --out prepare`: two sources tagged `Example Folder`, no warnings.
- **Import in the UI.** Open the account menu, click `Import`. `click "getByRole('button', { name: 'Choose OPML file' })"` reports a file chooser; `upload sources.opml`. Dialog `Import 2 sources?`; `screenshot --filename=import-confirm.png`; click `Import sources`. Poll `snapshot` for the retained `Subscriptions saved` dialog and `2 added, 0 updated, 0 already up to date.`; `screenshot --filename=import-done.png`. `inspect`: two `user_subscriptions` rows on the fixture URLs; `sqlite3 -readonly` on `user_subscriptions.tags` shows `["Example Folder"]`.
- **Collection feedback.** Wait until `GET /api/jobs/active` is `null`. Expand `Posts not fetching for 2 sources`: it lists only both fixture feed names, without per-feed timestamps or retry details. Save `import-collection.png`; click the `View sources` button and assert navigation to `/sources` with no open dialog. Repeat with one unavailable feed to prove singular wording, and with only the standing reachable feed from [subscriptions](./subscriptions.md) to prove there is no warning accordion, even when other subscriptions are unavailable. Check the collapsed and expanded receipt at a narrow viewport for overflow.
- **Idempotent.** `$V api $S POST /api/subscriptions/import --data "$(jq -c '{sources}' $E/prepare.json)" --out reimport`: `unchanged` 2, no new rows.
- **Export in the UI.** `goto /settings/export`, click `Download OPML`: the driver reports `Downloaded file morgenblau.opml` into `$E/.playwright-cli/`; it holds an `Example Folder` outline with both fixture feeds. `screenshot --filename=export-done.png`.
- **Export API.** `POST /api/subscriptions/export --out export`: `opml` contains both fixture URLs.
- **Other apps.** Click `Import from Skyreader`: either a toast `No sources to import` or a dialog listing the account's Skyreader feeds. Close it with `Cancel` or `press Escape`; never confirm, since those would be real subscriptions outside the fixture.
- **Negative.** `prepare` with `{"provider":"other"}`: `400`; with `{"provider":"opml","opml":"not xml"}`: `400` `Choose a valid OPML file with a body element`. `import` with `{"sources":[]}`: `400` `Import between 1 and 5 sources per batch`. `export --anon`: `401`.

## Evidence

- `sources.opml`, `prepare.json`, `reimport.json`, `export.json`, `.playwright-cli/morgenblau.opml`
- `import-confirm.png`, `import-done.png`, `import-collection.png`, `export-done.png`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- Imports are real writes to the run's own PDS, deleted with it by `down`.
- `upload` only takes files inside the evidence dir, so keep the fixture there.
- The import endpoint caps a batch at 5 sources to finish within the server's write deadline; a bigger fixture is split by the page, not by the API.
