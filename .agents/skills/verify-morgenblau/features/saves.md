# URL saves

A reader saves a web URL (usually an entry's link) for later. The save is a record on the reader's PDS, mirrored into the local index, and listed together with private newsletter saves. Saving the same URL twice returns the existing save.

Help article: none

## Sub-features

- `save` `POST /api/saves` with `{itemUrl, feedUrl}` answers `201` with the save; repeating it answers `200` with the same `rkey` (that body has no `cid`).
- `list` `GET /api/saves` lists URL saves (`rkey`, `itemUrl`) and newsletter saves (`kind` `newsletter`, `id`, `entrySlug`, no `rkey`).
- `unsave` `DELETE /api/saves/{rkey}`.
- `tag` `PATCH /api/saves/{rkey}` with `{tags}` rewrites the PDS record's tags, keeping its other fields; `PATCH /api/newsletter-saves/{id}` does the same for a private newsletter save, which never reaches the PDS. `GET /api/saves/tags` suggests the tags on both kinds. Saves carry `tags` in `GET /api/saves` and in the reader's `savedState`.
- `library` `/library` shows the 10 newest saves (Recent) and `/library?view=everything` every save, grouped by month, or by first letter (`&sort=title`) or site (`&sort=site`), filtered by the search in `&q=` (`matchesQuery` in `frontend/src/lib/library.ts`) (SPEC `<library>`). `GET /api/saves` gives a cached entry's `source`, `contentType` and `minutes`, and a newsletter save's `messageId`, `source` and `minutes`. The reader's `savedState` carries `savedAt`.

## How to reach it

- Browser: the reader's bookmark and end-of-article filing block, see [entry-reader](./entry-reader.md).
- Browser: `/library`, its header links `Recent` and `Everything`, the Everything search (the `Search your Library` icon next to the sort, key `/`), each row's hover actions (`Copy link`, `Remove from Library`; keys `C` and `⌫` on the selected row) and its context menu (right click, or long press on touch).
- API routes above.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`. An entry URL from the digest (`$V api $S GET /api/digest` gives `entries[].url`), or `https://example.com/verify-<slug>` when the change does not care which URL.

- **Save.** `$V api $S POST /api/saves --data '{"itemUrl":"<url>"}' --out save`: `201` with an `rkey`. `inspect`: a `user_saves` row with that rkey and URL.
- **Idempotent.** Repeat the POST: `200`, same `rkey`, still one row.
- **List.** `GET /api/saves --out saves`: contains the URL. With a newsletter save from [newsletters](./newsletters.md) in place, it also holds that entry with `kind` `newsletter` and no `rkey`.
- **Tag.** `PATCH /api/saves/<rkey> --data '{"tags":["read-later"]}'`: `200` with `tags`; `sqlite3 -readonly $E/run/morgenblau.db "select tags from user_saves"` shows them (`inspect` lists no tags) and `GET /api/saves/tags` lists `read-later`.
- **Library.** Save eleven digest entries and open `/library`: the header shows `Recent` and `Everything`, and Recent lists the ten newest, each with when it was saved. `Everything` lists all eleven under month headings; `Title` and `Site` regroup them by first letter and by source. Hover a row: `Copy link` and `Remove from Library` appear; right click shows `Open original`, `Copy link` and `Remove from Library`. `Remove from Library` deletes the row and its `Undo` saves it again with its tags (a new `rkey`; a newsletter save by its `messageId`) (`library-*.png`).
- **Library search.** Tag one save `design` and another `design-notes`. On `/library?view=everything` no search field shows until `Search your Library` (the icon after `Site`) is pressed; it opens a field under the sort row with focus in it, and Esc clears the search and closes the field, returning focus to the icon. On `/library`, press `/`: the URL becomes `?view=everything` and the field opens with focus. Loading a URL with `&q=` opens the field with the query but leaves focus on the page. Type `zurich creme`: only `Grüße aus Zürich, café crème` lists and the count reads `1 of <n> saves`. `#design` lists only the save tagged `design`. Choose `Site`, open the row, and the reader's `Back to library` returns to `?view=everything&sort=site&q=%23design` with the query still in the field. `quokka` reads `Nothing matches “quokka”.` (`search-*.png`).
- **Unsave.** `DELETE /api/saves/<rkey>`: `204`; the row is gone.
- **Negative.** `POST /api/saves --data '{}'`: `400` (`itemUrl is required`). `DELETE /api/saves/unknownrkey`: `404`. `GET /api/saves --anon`: `401`.

## Evidence

- `save.json` + `.status`, `saves.json`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- Saves are real writes to the run's own PDS, deleted with it by `down`.
- A newsletter message's public web URL is still saved privately through [newsletters](./newsletters.md), never through this route.
