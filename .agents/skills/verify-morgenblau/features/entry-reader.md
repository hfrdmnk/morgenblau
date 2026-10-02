# Entry reader

Clicking a digest item opens `/entry/<slug>?from=<day>`, an in-app reader for feed entries and private newsletters. Blog posts can load the full article from the original page. Newsletters render inline images through a private asset route and keep remote images blocked until the reader allows them for that message.

Help article: none

## Sub-features

- `reader` `/entry/<slug>` (`frontend/src/pages/entry.tsx`) shows the title, source line, body and an original link (`Read original` and others, see `sourceActionLabel`).
- `back` the `Reader` navigation's `Back to digest` returns to `/?date=<from>`.
- `extract` a blog post with a web URL shows `Load full article`; it calls `POST /api/entries/{slug}/extract`, which fetches the page, stores `feed_entries.extracted_body` and returns the entry. The page also extracts on load when the entry has no body.
- `inline-assets` newsletter inline (CID) images load from `GET /api/newsletter-assets/{token}`.
- `remote-images` a newsletter with remote images shows `Remote images are blocked` and `Load images`, which calls `POST /api/newsletters/messages/{id}/images`.
- `unavailable` an unknown slug shows `Entry unavailable`.

## How to reach it

- Browser: a digest item link on `/`, or `goto /entry/<entrySlug>` with a slug from `GET /api/digest`.
- API: `GET /api/entries/{slug}`, `POST /api/entries/{slug}/extract`, `GET /api/newsletter-assets/{token}`, `POST /api/newsletters/messages/{id}/images`.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, a newsletter delivered today as in [digest](./digest.md), browser signed in. A blog post entry for `extract` comes from the dev account's subscriptions (`GET /api/digest`, `contentType` `blogpost` with a `url`); with none today, try an earlier `date`, else that sub-feature is `blocked`.

- **Newsletter.** On `/`, `click "getByRole('link', { name: 'Morgenblau local newsletter sample' })"`: lands on `/entry/<entrySlug>?from=<today>`; `snapshot` shows the heading, `Remote images are blocked` and button `Load images`. `eval "() => [...document.querySelectorAll('article img')].map(i => [i.getAttribute('src'), i.naturalWidth])"`: the inline image is `/api/newsletter-assets/<token>` with a width above 0. `screenshot --filename=reader-newsletter-blocked.png`.
- **Load images.** Click `Load images`: the notice is gone, the remote `img` now has its `src`, and `inspect` shows `remote_images_allowed` 1. `screenshot --filename=reader-newsletter-images.png`.
- **Asset route.** `curl -b $E/run/cookies.txt <base_url>/api/newsletter-assets/<token>`: `200 image/png`; without the cookie `401`; an unknown token `404`. Keep the lines in `asset.txt`.
- **Back.** Click `Back to digest`: ends on `/?date=<today>`.
- **Full article.** Open the blog post's entry, note `sqlite3 -readonly $E/run/morgenblau.db "select length(coalesce(extracted_body,'')) from feed_entries where entry_slug='<slug>'"`, click `Load full article`: the button is gone, the body grows and the stored length is above 0. `screenshot --filename=reader-article.png`. `$V api $S POST /api/entries/<slug>/extract --out extract`: `200` with the extracted `body`.
- **Negative.** `goto /entry/unknownslug`: `Entry unavailable`. `POST /api/entries/unknownslug/extract` and `POST /api/entries/<newsletter entrySlug>/extract`: `404` (newsletters are never extracted). `POST /api/entries/<slug>/extract --anon`: `401`.

## Evidence

- `reader-newsletter-blocked.png`, `reader-newsletter-images.png`, `reader-article.png`, `reader-unknown.png`
- `asset.txt`, `extract.json`, `extract-db.txt` (before and after lengths)
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- Extraction fetches the entry's original page from the public internet through the SSRF-guarded client; an unreachable site answers `502` and the page says `The full article could not be loaded`. That is `blocked` for the drive, not a product failure.
- An extracted body is cached in the run DB, so a second `extract` returns it without fetching.
- Tenant checks for another reader's entry live in [tenant-isolation](./tenant-isolation.md).
