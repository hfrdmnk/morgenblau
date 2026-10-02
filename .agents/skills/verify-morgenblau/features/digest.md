# Daily digest

The signed-in home screen. It shows one day's entries from the reader's subscriptions and private newsletters, today by default, with a date strip in the app header for moving between days. Each item opens its entry in the [reader](./entry-reader.md).

Help article: none

## Sub-features

- `digest-today` `/` lists today's entries, or `Nothing was published here today.` when empty, or `New entries are still being collected.` while a sync runs.
- `digest-date` `/?date=YYYY-MM-DD` shows that day; the UI shows today for a future date.
- `date-nav` the header's `Digest date` navigation (`DigestDateNav` in `frontend/src/components/app-header.tsx`, only on `/`) links three days before the selected day and up to three after it, never past today (`digestDateRange` in `frontend/src/lib/digest-dates.ts`); the selected one has `aria-current="date"`.
- `digest-api` `GET /api/digest?date=&timezone=` returns `{date, entries, hasActiveJob}`. It does not clamp a future date (it answers that day, empty); a bad `date` or unknown `timezone` is `400`.
- `entry-api` `GET /api/entries/{slug}` returns one entry (feed entry or newsletter message) for the reader.

## How to reach it

- Browser: `/`, `/?date=<day>`, the links in the header's `Digest date` (accessible names are full dates in the browser's en-US locale, such as `Tuesday, September 29, 2026`; `LC_ALL=en_US.UTF-8 date -v-1d '+%A, %B %-d, %Y'` prints yesterday's).
- API: `GET /api/digest`, `GET /api/entries/{slug}`.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`, browser signed in (see [sign-in](./sign-in.md)).
- Digest content: a newsletter delivered today is the local, PDS-free way to get an entry. Create the address and deliver as in [newsletters](./newsletters.md) (`POST /api/newsletters/address`, `$V mail`), wait about 5 seconds for the processor.

- **Empty day.** Before any delivery, `$V api $S GET "/api/digest?timezone=Europe/Zurich" --out digest-empty`: `entries` is empty, since the run's account starts with no subscriptions.
- **Entry appears.** After delivery, sign the browser in (`/login`, `Log me in`), `goto /`, `find "Morgenblau local newsletter sample"`: a link to `/entry/<entrySlug>?from=<today>` with the sender line `sample@sender.example`. `screenshot --filename=digest-today.png`.
- **API agrees.** `$V api $S GET "/api/digest?timezone=Europe/Zurich" --out digest` holds an entry with `contentType` `newsletter`, `title` `Morgenblau local newsletter sample`, and an `entrySlug`.
- **Entry detail.** `$V api $S GET /api/entries/<entrySlug> --out entry`: `200` with the same title.
- **Date navigation.** `click "getByRole('link', { name: '<yesterday full date>' })"`, then `eval "() => location.search"`: `?date=<yesterday>`; the newsletter item is gone; `screenshot --filename=digest-yesterday.png`. `goto "/?date=2999-01-01"`: the heading shows today's weekday and `eval "() => document.querySelector('[aria-current=date]')?.getAttribute('aria-label')"` is today. `$V api $S GET "/api/digest?date=2999-01-01&timezone=Europe/Zurich" --out digest-future`: `200`, `date` `2999-01-01`, no entries.
- **Negative.** `$V api $S GET "/api/digest?date=not-a-date"`: `400`. `GET "/api/digest?timezone=Mars/Olympus"`: `400` `invalid timezone`. `$V api $S GET /api/entries/unknownslug`: `404`.

## Evidence

- `digest-empty.json`, `digest.json`, `digest-future.json`, `entry.json`
- `digest-today.png`, `digest-yesterday.png`
- `inspect-before.json`, `inspect-after.json`

## Gotchas

- The digest groups by the browser's local day. Pass `timezone` explicitly on API calls and use the same zone as the browser, or a late-evening delivery lands on another day.
- Feeds the run adds bring real entries next to the fixture. Assert on the fixture's title, not on counts.
