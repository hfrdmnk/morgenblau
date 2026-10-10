# Generated feeds

When a pasted page has no feed, the `Add a source` dialog offers to generate one. The server reads the listing page and serves the posts it finds as RSS at the public `/web/<id>`; the reader subscribes to that URL like any RSS feed, so the PDS record is an ordinary `#rssFeed`. When the generator is unsure, the reader can point at one post and generate again. A generated feed's health shows on the Sources page and never in the newspaper.

Help article: none

## Sub-features

- `offer` (`GenerateFeed` in `frontend/src/components/generate-feed.tsx`, rendered by `ResolvedSources` when resolve finds no candidates). It shows `No feed found on this page…` and `Generate a feed`. While it waits, a status says it `can take a little while`.
- `preview` shows the site title and host, the first posts with their dates when the page has them (fewer when unsure, `PREVIEW_POSTS`), and `<n> posts found`. Focus lands on the preview heading, and the next Tab reaches `Add source` (`focusOrder` in `generate-feed.tsx`). `Add source` posts `{feedUrl, title, siteUrl}` to `POST /api/subscriptions` and toasts `Source added`.
- `point-at-a-post`: when `needsAnchor` is set (derived from `setup_health = needs_attention`; there is no column) or health is not `ok`, an extra field asks for one post (`ANCHOR_PROMPT` in `generate-feed.tsx`: `Morgenblau isn’t sure these are the site’s posts…`, or `No posts found on this page yet…` when nothing was found), and `Use this post` posts `anchorUrl` again. With posts found, the preview and `Add source` stay available; with none, only the field shows. Server failures show without marking the field invalid.
- `blocked`: `blocked_by_site` reads `This site blocks feed readers.` and offers neither `Add source` nor the field.
- `failures`: `429` reads `Too many generated feeds this hour. Try again later.` with no retry; `502` reads `Couldn’t read that page.` with `Try again`. `generationError` and `canRetryGeneration` in `frontend/src/lib/add-source.ts` own the wording and which failures offer a retry. An unreachable host never gets this far in the dialog: resolve already answers `Couldn't reach that URL`.
- `create` `POST /api/web-sources` `{url, anchorUrl?}` (`internal/api/web_sources.go`, `internal/websource`). `Service.Create` owns when a site's existing source is reused and when setup runs again (an unsure setup last tried over `staleSetup` ago, or an `anchorUrl` on a source that is not settled).
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
- The generator is always wired (`newWebGenerator` in `internal/server/web_sources.go`); the model steps run only when the server's environment has `ANTHROPIC_API_KEY`, and startup logs `feed generation model=<bool>`. To arrange a state without fetching, insert a row the service reuses: `listing_key` is `listingKey` of the pasted URL (host without `www.`, then the path without its trailing slash, e.g. `example.com` or `example.com/blog`), and `setup_tried_at` is now, so an unsure row is not regenerated. `health`/`health_reason` are the source now, `setup_health`/`setup_health_reason` the setup in force; `setup_seq` defaults to 0:
  `sqlite3 $E/run/morgenblau.db "insert into web_sources (id, listing_key, listing_url, site_url, title, recipe, health, health_reason, created_by_did, created_at, updated_at, setup_at, setup_health, setup_health_reason, setup_tried_at) values ('verifyok', 'example.com', 'https://example.com/', 'https://example.com/', 'Example Publication', '{}', 'ok', '', 'did:plc:verify', '<now>', '<now>', '<now>', 'ok', '', '<now>')"`. Then insert a few `web_source_items` rows (`source_id, url, title, published_at or NULL, first_seen_at, history=1, position`).

Steps:

- **Offer.** Find feeds for `example.com`: `Generate a feed` instead of `Available feeds`. `screenshot --hires --filename=generate-offer.png`.
- **Generate.** Click `Generate a feed`. The pending status, then a preview within about 20 s (`generate-pending.png`).
- **Preview and add.** The title, `example.com`, five posts (dates only where stored), and `12 posts found` (`generate-preview.png`). Keyboard: `document.activeElement` is the preview heading, and one Tab reaches `Add source`. Click `Add source` and wait for `GET /api/jobs/active` to be `null`. `inspect`: one `user_subscriptions` row on `<base_url>/web/<id>`. `GET /api/subscriptions --out subscriptions-generated` returns `generated: {"health":"ok"}` and `fetchStatus: "ready"`. `feeds` holds 12 entries, fetched in process.
- **Sources health.** `/sources` shows `Generated feed` (`sources-generated-ok.png`). Set `health='needs_attention', health_reason='<reason>'` on the row. Within one 15 s poll the line reads `Generated feed · Needs attention: <reason>` (`sources-needs-attention.png`, plus a 390 px wide shot with no horizontal overflow). Then `blocked_by_site` reads `This site blocks feed readers` (`sources-blocked.png`).
- **Point at a post.** Arrange an unhealthy row for `example.org` (`health` and `setup_health` both `needs_attention`, a few items), then generate: the preview plus the `Morgenblau isn’t sure these are the site’s posts…` field, with `Add source` enabled (`generate-unsure.png`, plus a 390×844 shot). Fill `example.org/posts/3` and click `Use this post`. With the generator, a new preview replaces the old one. Without it, `Feed generation isn't available yet` appears under the field, the field is not `aria-invalid`, and the preview and `Add source` remain (`generate-anchor-unavailable.png`).
- **Blocked.** Arrange `blocked_by_site` for `example.net` and generate: `This site blocks feed readers.`, with no `Add source` and no field (`generate-blocked.png`).
- **Rate limit.** Each create that reaches the limiter counts, including `503`s: 20 per reader per hour (`server.go`). Call `POST /api/web-sources` with fresh `example.com/limit-<n>` URLs until one answers `429 rate_limited`. Then generate in the dialog: `Too many generated feeds this hour. Try again later.` with no `Try again` (`generate-rate-limited.png`).
- **Poll and repair.** After subscribing, set `next_poll_at` to a past time on the run DB; within a minute `last_polled_at` and `link_set_hash` are set and an `ok` poll adds a `web_source_polls` row whose `date_frac` matches the setup's row, no item loses its `published_at`, and a second forced poll of the unchanged page adds none. To drive a repair, change the stored recipe's `streams[0].url_template` to a shape the page lacks and clear `link_set_hash`: the next poll records a `web_source_repairs` row (`known_urls`, `false_alarm`) and the recipe's template is restored.
- **502.** A page whose fetch fails answers `502 upstream_error` (`POST /api/web-sources` for an unreachable host shows it), and the dialog reads `Couldn’t read that page.` with `Try again`.

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

- `Service.Create` returns an unsure row as-is only while `setup_tried_at` is younger than `staleSetup` (10 minutes); after that, generating again calls the generator. Re-arrange or update `setup_tried_at` for long runs.
- Vite hot reload can close the dialog; drive a state again from `Find feeds` after editing the frontend.
- The fetcher answers this server's own `/web/` feeds in process (`ownWebFeeds`), so a loopback `base_url` feed is not refused by the SSRF guard.
