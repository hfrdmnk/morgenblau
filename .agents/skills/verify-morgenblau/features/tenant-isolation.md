# Reader isolation

Every reader's data is scoped by DID. Another reader's newsletter sources, messages and saves are invisible: the API answers 404 for them on every verb, never 403, and anonymous callers get 401. PDS-backed subscriptions and saves are scoped the same way in SQL.

Help article: none

## Sub-features

- `foreign-newsletter-404` the dev account gets 404 for a second reader's source (read, edit, stop, enable), source entries, message entry and extract, image consent, move, save, unsave and inline asset.
- `foreign-not-listed` the second reader's source is absent from the dev account's `GET /api/newsletters` and digest.
- `anon-401` the same URLs answer 401 without a session.

## How to reach it

- API: the `/api/newsletters/*`, `/api/newsletter-saves*`, `/api/newsletter-assets/*` and `/api/entries/*` routes in `internal/server/routes.go`.
- Browser: `/entry/<slug>` of the second reader's message.

## Drive and prove

Preconditions:

- Instance up. `$V foreign $S > $E/foreign.json` creates a second reader (placeholder DID) with a newsletter address; `$V mail $S -to "$(jq -r .address $E/foreign.json)"` delivers to it through the real SMTP path. Wait about 5 seconds.
- `$V inspect $S --out inspect-foreign`: the second reader's `newsletter_sources` id and `newsletter_messages` id and `entry_slug`. Its inline asset token: `sqlite3 -readonly $E/run/morgenblau.db "select token from newsletter_inline_assets where message_id='<messageId>'"`.
- For the unsave probe, arrange a save owned by the second reader: `sqlite3 $E/run/morgenblau.db "insert into newsletter_saves (id, did, message_id, created_at) values ('01EXAMPLEFOREIGNSAVE000000', '<foreign did>', '<messageId>', '2026-01-01T00:00:00Z')"`.

- **Anonymous.** `$V api $S GET /api/newsletters/<sourceId> --anon --out anon-foreign-source` and `GET /api/newsletter-assets/<token> --anon --out anon-foreign-asset`: `401`. `$V browser $S open /entry/<entrySlug>`: ends on `/login`; `screenshot --filename=entry-anon.png`.
- **Signed in (doctor exit 0).** After `$V login $S`, each of these answers `404`:
  - `GET /api/newsletters/<sourceId> --out foreign-source`
  - `GET /api/newsletters/<sourceId>/entries`
  - `GET /api/entries/<entrySlug>`
  - `POST /api/newsletters/messages/<messageId>/images`
  - `POST /api/newsletters/messages/<messageId>/move --data '{"newSourceTitle":"Example"}'`
  - `POST /api/newsletter-saves --data '{"messageId":"<messageId>"}'`
  - `PATCH /api/newsletters/<sourceId> --data '{"title":"Example"}'`, `POST /api/newsletters/<sourceId>/stop`, `POST .../enable`
  - `POST /api/entries/<entrySlug>/extract`
  - `GET /api/newsletter-assets/<token>`
  - `DELETE /api/newsletter-saves/01EXAMPLEFOREIGNSAVE000000`
  Collect the statuses in `probes.txt`.
- **Not listed.** `GET /api/newsletters` and `GET /api/digest` do not mention the second reader's source or message.
- **Unchanged.** `$V inspect $S --out inspect-after`: the second reader's message still has `remote_images_allowed` 0 and the same `source_id`, its source keeps its title and `active` status, and its arranged save is the only `newsletter_saves` row.

## Evidence

- `foreign.json`, `mail.log`, `inspect-foreign.json`, `inspect-after.json`
- `anon-foreign-source.json` + `.status`, `anon-foreign-asset.status`, one `.status` per signed-in probe, `probes.txt`
- `entry-anon.png`

## Gotchas

- The dev account is the only real signed-in reader, so the second reader exists only in the run DB. It cannot sign in; isolation is proven from the dev account's side.
- `$V foreign` and the arranged save are the only direct DB writes; they set up the second reader. Everything after them goes through SMTP and the API.
