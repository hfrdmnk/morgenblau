# Generated feeds

When a pasted page has no feed, the `Add a source` dialog offers to generate one. The server reads the listing page and serves the posts it finds as RSS at the public `/web/<id>`; the reader subscribes to that URL like any RSS feed, so the PDS record is an ordinary `#rssFeed`. When the generator is unsure, the reader can point at one post and generate again. A generated feed's health shows on the Sources page and never in the newspaper.

Help article: none

## Sub-features

- `offer` (`GenerateFeed` in `frontend/src/components/generate-feed.tsx`, rendered by `ResolvedSources` when resolve finds no candidates). It shows `No feed found on this page…` and `Generate a feed`. While it waits, a status says it `can take a little while`.
- `preview` shows the site title and host, the first posts with their dates when the page has them, and `<n> posts found`. `Add source` posts `{feedUrl, title, siteUrl}` to `POST /api/subscriptions` and toasts `Source added`.
- `point-at-a-post`: when `needsAnchor` is set or health is not `ok`, an extra field asks for one post (`This list looks unsure…`, or `No posts found…` when nothing was found), and `Use this post` posts `anchorUrl` again. The preview and `Add source` stay available. Server failures show without marking the field invalid.
- `blocked`: `blocked_by_site` reads `This site blocks feed readers.` and offers neither `Add source` nor the field.
- `failures`: `429` reads `Too many generated feeds this hour. Try again later.` with no retry, `502` reads `Couldn’t reach that site. Try again later.`, and any other status shows the server's message with `Try again`. `generationError` in `frontend/src/lib/add-source.ts` owns this wording.
- `create` `POST /api/web-sources` `{url, anchorUrl?}` (`internal/api/web_sources.go`, `internal/websource`). It reuses a site's existing source unless that source is unhealthy and older than `staleSetup`, or an `anchorUrl` is given.
- `health in Sources`: `GET /api/subscriptions` adds `generated: {health, healthReason}` for subscriptions on this server's own `/web/` feeds (the join in `ListUserSourcesWithStats`). `/sources` shows `Generated feed`, `Generated feed · Needs attention: <reason>`, or `This site blocks feed readers` under the URL (`GeneratedNote` in `frontend/src/components/feed-health.tsx`).
- `feed` `GET /web/<id>` is public RSS ([public-pages](./public-pages.md)).
- `poll and repair`: `runWebPoll` polls followed sources every `WEB_POLL_INTERVAL_MINUTES` through `WebfeedGenerator.Poll` (`internal/websource/webfeedgen.go`; detector and ladder in `internal/webfeed/detect.go` and `repair.go`). State lands in `web_sources` (`last_polled_at`, `link_set_hash`, `health`), `web_source_polls` (good polls only) and `web_source_repairs`.

## How to reach it

- Browser: `Add a source`, paste a page that has no feed, `Find feeds`, `Generate a feed`. Then `/sources`.
- API: the routes above (`internal/server/routes.go`).

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, browser signed in (open with `--device "Desktop Chrome HiDPI"` for 2x screenshots). Network access for resolve.
- Pages without a feed: `example.com`, `example.org`, `example.net`. Feed-backed control: the standing feed site from [subscriptions](./subscriptions.md).
- The generator is always wired (`newWebGenerator` in `internal/server/web_sources.go`); the model steps run only when the server's environment has `ANTHROPIC_API_KEY`, and startup logs `feed generation model=<bool>`. To arrange a state without fetching, insert a row the service reuses: `listing_key` is the origin with no trailing slash, and `setup_at` is now, so an unhealthy row is not regenerated:
  `sqlite3 $E/run/morgenblau.db "insert into web_sources (id, listing_key, listing_url, site_url, title, recipe, health, health_reason, needs_anchor, created_by_did, created_at, updated_at, setup_at) values ('verifyok', 'https://example.com', 'https://example.com/', 'https://example.com/', 'Example Publication', '{}', 'ok', '', 0, 'did:plc:verify', '<now>', '<now>', '<now>')"`. Then insert a few `web_source_items` rows (`source_id, url, title, published_at or NULL, first_seen_at, history=1, position`).

Steps:

- **Offer.** Find feeds for `example.com`: `Generate a feed` instead of `Available feeds`. `screenshot --hires --filename=generate-offer.png`.
- **Generate.** Click `Generate a feed`. The pending status, then a preview within about 20 s (`generate-pending.png`).
- **Preview and add.** The title, `example.com`, five posts (dates only where stored), and `12 posts found` (`generate-preview.png`). Click `Add source` and wait for `GET /api/jobs/active` to be `null`. `inspect`: one `user_subscriptions` row on `<base_url>/web/<id>`. `GET /api/subscriptions --out subscriptions-generated` returns `generated: {"health":"ok"}` and `fetchStatus: "ready"`. `feeds` holds 12 entries, fetched in process.
- **Sources health.** `/sources` shows `Generated feed` (`sources-generated-ok.png`). Set `health='needs_attention', health_reason='<reason>'` on the row. Within one 15 s poll the line reads `Generated feed · Needs attention: <reason>` (`sources-needs-attention.png`, plus a 390 px wide shot with no horizontal overflow). Then `blocked_by_site` reads `This site blocks feed readers` (`sources-blocked.png`).
- **Point at a post.** Arrange an unhealthy row for `example.org` (`needs_attention`, `needs_anchor=1`, a few items), then generate: the preview plus the `This list looks unsure…` field, with `Add source` enabled (`generate-unsure.png`). Fill `example.org/posts/3` and click `Use this post`. With the generator, a new preview replaces the old one. Without it, `Feed generation isn't available yet` appears under the field, the field is not `aria-invalid`, and the preview and `Add source` remain (`generate-anchor-unavailable.png`).
- **Blocked.** Arrange `blocked_by_site` for `example.net` and generate: `This site blocks feed readers.`, with no `Add source` and no field (`generate-blocked.png`).
- **Rate limit.** Each create that reaches the limiter counts, including `503`s: 20 per reader per hour (`server.go`). Call `POST /api/web-sources` with fresh `example.com/limit-<n>` URLs until one answers `429 rate_limited`. Then generate in the dialog: `Too many generated feeds this hour. Try again later.` with no `Try again` (`generate-rate-limited.png`).
- **Poll and repair.** After subscribing, set `next_poll_at` to a past time on the run DB; within a minute `last_polled_at` and `link_set_hash` are set and an `ok` poll adds a `web_source_polls` row, while a second forced poll of the unchanged page adds none. To drive a repair, change the stored recipe's `streams[0].url_template` to a shape the page lacks and clear `link_set_hash`: the next poll records a `web_source_repairs` row (`known_urls`, `false_alarm`) and the recipe's template is restored.
- **502.** A page whose fetch fails answers `502 upstream_error`, and the dialog reads `Couldn’t reach that site. Try again later.` with `Try again`.

Negative proofs:

- The newspaper stays quiet. While the source needs attention, `GET /api/digest?date=<a day with its posts>` contains no health fields, and `/` shows the posts with no attention text (`digest-quiet.png`).
- A feed-backed site never offers generation: resolve lists `Available feeds` and `Generate a feed` is absent (`feed-site-no-offer.png`).
- `POST /api/web-sources --anon`: `401`. `--data '{}'`: `400 invalid_request`. `{"url":"javascript:alert(1)"}`: `400`, and nothing is generated or rate-counted.
- `GET /web/unknown --anon`: `404`.

## Evidence

- `notes.md` (generator wired or not, rows arranged), and the screenshots above
- `subscriptions-generated.json`, `web-feed-anon.json`, `digest-quiet.json`, `limit-<n>.json`, `web-sources-*.json`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- `Service.Create` returns an unhealthy row as-is only while `setup_at` is younger than `staleSetup` (10 minutes); after that, generating again calls the generator. Re-arrange or update `setup_at` for long runs.
- Vite hot reload can close the dialog; drive a state again from `Find feeds` after editing the frontend.
- The fetcher answers this server's own `/web/` feeds in process (`ownWebFeeds`), so a loopback `base_url` feed is not refused by the SSRF guard.
