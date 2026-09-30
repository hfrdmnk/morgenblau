# Subscriptions

A reader follows RSS or Atom feeds and Standardfeed publications. Adding one writes a subscription record to the reader's PDS first and then mirrors it into the local index; a fetch job loads its entries. Readers can rename, tag, mark primary, change the feed URL, or remove a subscription. The Sources screen is not built yet, so the API is the entry point.

Help article: none

## Sub-features

- `resolve` `POST /api/subscriptions/resolve` with `{url}` finds feed candidates on a page or feed URL.
- `add` `POST /api/subscriptions` with `{"subscriptions":[{"feedUrl":"..."}]}` (RSS) or `{"publication":"at://..."}` (Standardfeed); returns `{records, jobIds}`.
- `list` `GET /api/subscriptions`, `GET /api/subscriptions/{rkey}`, `GET /api/subscriptions/{rkey}/entries`, `GET /api/subscriptions/tags`.
- `edit` `PATCH /api/subscriptions/{rkey}` with any of `title`, `primary`, `tags`, `feedUrl`.
- `remove` `DELETE /api/subscriptions/{rkey}`.

## How to reach it

- API routes above. No UI entry point yet (the digest's `Add a source` button has no handler).

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S` (records the PDS baseline). Network access to the dev account's PDS and to the feed.
- A public feed URL for this run: the task's, else the user's own `https://dominikhofer.me/rss`, which they approved as the standing test feed. Record the one used in `$E/notes.md`. A loopback fixture feed is refused by the SSRF guard, and an add writes the PDS record before any fetch, so never probe with a bad feed URL.

- **Resolve.** `$V api $S POST /api/subscriptions/resolve --data '{"url":"<page or feed url>"}' --out resolve`: `200` with at least one candidate.
- **Add.** `$V api $S POST /api/subscriptions --data '{"subscriptions":[{"feedUrl":"<feed url>"}]}' --out add`: `records[0].rkey` and a `jobIds` entry. Wait until `GET /api/jobs/active` is empty.
- **Mirrored.** `$V inspect $S`: a `user_subscriptions` row with that rkey. `GET /api/subscriptions/<rkey>/entries --out entries`: fetched entries.
- **Edit.** `PATCH /api/subscriptions/<rkey> --data '{"title":"Example Feed","tags":["example"]}' --out patch`: `200`; `GET /api/subscriptions/tags` includes `example`.
- **Remove.** `DELETE /api/subscriptions/<rkey>`: success; the row is gone from `inspect`, and `down` then reports nothing left to delete for it.
- **Negative.** `PATCH /api/subscriptions/unknownrkey --data '{"title":"x"}'`: `404`. `GET /api/subscriptions --anon`: `401`.

## Evidence

- `notes.md` (the feed URL used), `resolve.json`, `add.json`, `entries.json`, `patch.json`
- `inspect-before.json`, `inspect-after.json`, `cleanup.log`

## Gotchas

- These are real PDS writes on the dev account. `down` deletes subscriptions whose rkey was not in the login baseline; run `$V login $S` before any add, and `down` while the instance is still running.
- Standardfeed adds resolve the publication's identity over the network; a failure there is `blocked`, not `fail`.
- A session without the Standardfeed scope gets `403` with `code: "reauth_required"` on Standardfeed writes; the dev login's password session is not scoped this way.
