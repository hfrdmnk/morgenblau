# Daily digest

The signed-in home screen. It shows one day's entries from the reader's subscriptions and private newsletters, newest day by default, with a seven-day date strip for going back. Each item links to its entry.

Help article: none

## Sub-features

- `digest-today` `/` lists today's entries, or `Nothing was published here today.` when empty, or `New entries are still being collected.` while a sync runs.
- `digest-date` `/?date=YYYY-MM-DD` shows that day; future dates fall back to today.
- `date-nav` the `Digest date` navigation links the last seven days; the selected one has `aria-current="date"`.
- `digest-api` `GET /api/digest?date=&timezone=` returns `{date, entries, hasActiveJob}`.
- `entry-api` `GET /api/entries/{slug}` returns one entry (feed entry or newsletter message) for the reader.

## How to reach it

- Browser: `/`, `/?date=<day>`, the links in `Digest date` (accessible names are full dates in the browser's en-US locale, such as `Tuesday, September 29, 2026`; `LC_ALL=en_US.UTF-8 date -v-1d '+%A, %B %-d, %Y'` prints yesterday's).
- API: `GET /api/digest`, `GET /api/entries/{slug}`.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, browser signed in (see [sign-in](./sign-in.md)).
- Digest content: a newsletter delivered today is the local, PDS-free way to get an entry. Create the address and deliver as in [newsletters](./newsletters.md) (`POST /api/newsletters/address`, `$V mail`), wait about 5 seconds for the processor.

- **Empty day.** Before any delivery, `$V api $S GET "/api/digest?timezone=Europe/Zurich" --out digest-empty`: `entries` is empty unless the dev account's subscriptions published today. Record which.
- **Entry appears.** After delivery, `$V browser $S goto /`, `find "Morgenblau local newsletter sample"`: the item is listed with the sender line `sample@sender.example`. `screenshot --filename=digest-today.png`.
- **API agrees.** `$V api $S GET "/api/digest?timezone=Europe/Zurich" --out digest` holds an entry with `contentType` `newsletter`, `title` `Morgenblau local newsletter sample`, and an `entrySlug`.
- **Entry detail.** `$V api $S GET /api/entries/<entrySlug> --out entry`: `200` with the same title.
- **Date navigation.** `click "getByRole('link', { name: '<yesterday full date>' })"`, then `eval "() => location.search"`: `?date=<yesterday>`; the newsletter item is gone. `goto "/?date=2999-01-01"`: the heading shows today's weekday and `eval "() => document.querySelector('[aria-current=date]')?.getAttribute('aria-label')"` is today.
- **Negative.** `$V api $S GET "/api/digest?date=not-a-date"`: `400`. `$V api $S GET /api/entries/unknownslug`: `404`.

## Evidence

- `digest-empty.json`, `digest.json`, `entry.json`
- `digest-today.png`, `digest-yesterday.png`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- The digest groups by the browser's local day. Pass `timezone` explicitly on API calls and use the same zone as the browser, or a late-evening delivery lands on another day.
- Clicking a digest item opens `/entry/<slug>`, which is still a placeholder page; prove the entry through the API.
- The dev account's real subscriptions sync on login, so entries from real feeds can appear next to the fixture. Assert on the fixture's title, not on counts.
