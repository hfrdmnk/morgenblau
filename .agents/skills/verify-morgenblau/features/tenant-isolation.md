# Reader isolation

Every reader's data is scoped by DID. Another reader's newsletter sources, messages and saves are invisible: the API answers 404 for them on every verb, never 403, and anonymous callers get 401. PDS-backed subscriptions and saves are scoped the same way in SQL.

Help article: none

## Sub-features

- `foreign-newsletter-404` the dev account gets 404 for a second reader's source, source entries, message entry, image consent, move and save.
- `foreign-not-listed` the second reader's source is absent from the dev account's `GET /api/newsletters` and digest.
- `anon-401` the same URLs answer 401 without a session.

## How to reach it

- API: `/api/newsletters/{id}`, `/api/newsletters/{id}/entries`, `/api/entries/{slug}`, `/api/newsletters/messages/{id}/images`, `/api/newsletters/messages/{id}/move`, `/api/newsletter-saves`.
- Browser: `/entry/<slug>` of the second reader's message.

## Drive and prove

Preconditions:

- Instance up. `$V foreign $S > $E/foreign.json` creates a second reader (placeholder DID) with a newsletter address; `$V mail $S -to "$(jq -r .address $E/foreign.json)"` delivers to it through the real SMTP path. Wait about 5 seconds.
- `$V inspect $S --out inspect-foreign`: the second reader's `newsletter_sources` id and `newsletter_messages` id and `entry_slug`.

- **Anonymous.** `$V api $S GET /api/newsletters/<sourceId> --anon --out anon-foreign-source`: `401`. `$V browser $S open /entry/<entrySlug>`: ends on `/login`.
- **Signed in (doctor exit 0).** After `$V login $S`, each of these answers `404`:
  - `GET /api/newsletters/<sourceId> --out foreign-source`
  - `GET /api/newsletters/<sourceId>/entries`
  - `GET /api/entries/<entrySlug>`
  - `POST /api/newsletters/messages/<messageId>/images`
  - `POST /api/newsletters/messages/<messageId>/move --data '{"newSourceTitle":"Example"}'`
  - `POST /api/newsletter-saves --data '{"messageId":"<messageId>"}'`
- **Not listed.** `GET /api/newsletters` and `GET /api/digest` do not mention the second reader's source or message.
- **Unchanged.** `$V inspect $S --out inspect-after`: the second reader's message still has `remote_images_allowed` 0 and the same `source_id`, and no `newsletter_saves` row exists for it.

## Evidence

- `foreign.json`, `mail.log`, `inspect-foreign.json`, `inspect-after.json`
- `anon-foreign-source.json` + `.status`, `foreign-source.status` and one `.status` per signed-in probe
- `entry-anon.png`

## Gotchas

- The dev account is the only real signed-in reader, so the second reader exists only in the run DB. It cannot sign in; isolation is proven from the dev account's side.
- `$V foreign` is the one sanctioned direct DB write: it arranges the second reader's address. Everything after it goes through SMTP and the API.
