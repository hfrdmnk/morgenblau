# URL saves

A reader saves a web URL (usually an entry's link) for later. The save is a record on the reader's PDS, mirrored into the local index, and listed together with private newsletter saves. Saving the same URL twice returns the existing save.

Help article: none

## Sub-features

- `save` `POST /api/saves` with `{itemUrl, feedUrl}` answers `201` with the save; repeating it answers `200` with the same `rkey` (that body has no `cid`).
- `list` `GET /api/saves` lists URL saves (`rkey`, `itemUrl`) and newsletter saves (`kind` `newsletter`, `id`, `entrySlug`, no `rkey`).
- `unsave` `DELETE /api/saves/{rkey}`.

## How to reach it

- API routes above. The Library page that will list saves is still a placeholder.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`. An entry URL from the digest (`$V api $S GET /api/digest` gives `entries[].url`), or `https://example.com/verify-<slug>` when the change does not care which URL.

- **Save.** `$V api $S POST /api/saves --data '{"itemUrl":"<url>"}' --out save`: `201` with an `rkey`. `inspect`: a `user_saves` row with that rkey and URL.
- **Idempotent.** Repeat the POST: `200`, same `rkey`, still one row.
- **List.** `GET /api/saves --out saves`: contains the URL. With a newsletter save from [newsletters](./newsletters.md) in place, it also holds that entry with `kind` `newsletter`; cleanup ignores it because it has no `rkey`.
- **Unsave.** `DELETE /api/saves/<rkey>`: `204`; the row is gone.
- **Negative.** `POST /api/saves --data '{}'`: `400` (`itemUrl is required`). `DELETE /api/saves/unknownrkey`: `404`. `GET /api/saves --anon`: `401`.

## Evidence

- `save.json` + `.status`, `saves.json`
- `inspect-before.json`, `inspect-after.json`, `cleanup.log`

## Gotchas

- Real PDS writes on the dev account; `down` deletes saves not in the login baseline.
- A newsletter message's public web URL is still saved privately through [newsletters](./newsletters.md), never through this route.
