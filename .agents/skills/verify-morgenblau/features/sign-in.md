# Sign in and out

A visitor who is not signed in lands on `/login`, which asks for an Atmosphere handle. In local development with a configured dev account, the page also offers `Log me in`, which signs in as that account and opens the digest. Signed-in readers log out from the account menu on the digest.

Help article: none

## Sub-features

- `login-page` `/login` renders the handle form.
- `anon-redirect` any app page redirects an anonymous visitor to `/login`; any `/api/*` call answers 401.
- `dev-login` `Log me in` signs in as the dev account and lands on `/`.
- `logged-in-redirect` a signed-in reader who opens `/login` goes to `/`.
- `logout` the account menu's `Log out` ends the session.

## How to reach it

- Browser: `/login`, or any app path while signed out.
- `Development login` panel on `/login`, button `Log me in` (only when the dev account is configured).
- Digest header, button `Open account menu`, then `Log out`.
- API: `GET /dev/login` reports availability, `POST /dev/login` signs in (the `$V login` lever).

## Drive and prove

Preconditions:

- Instance up. `dev-login`, `logged-in-redirect` and `logout` need doctor exit `0`.

- **Login page.** `$V browser $S open /login`, then `snapshot`. Heading `Sign in with your Atmosphere account`, textbox `Handle`, button `Continue`.
- **Anonymous redirect.** `$V browser $S goto /settings`, then `eval "() => location.href"`: ends on `/login`. `$V api $S GET /api/digest --anon --out anon-digest`: status `401`.
- **Dev login unavailable.** On a doctor-exit-3 run, `$V api $S GET /dev/login --anon --out dev-login`: `404`, and `find "Log me in"` on `/login` finds nothing.
- **Dev login.** `$V login $S` first (PDS baseline). Then on `/login`, `find "Development login"` shows the panel; `click "getByRole('button', { name: 'Log me in' })"`. Poll `eval "() => location.href"` until it is the instance root. `snapshot` shows the date navigation `Digest date` and the button `Open account menu`.
- **Signed-in redirect.** `goto /login`: ends on `/`.
- **Logout.** `click "getByRole('button', { name: 'Open account menu' })"`, `snapshot`: the popover shows `@<handle>` and `Log out`. Click `Log out`. `goto /`: ends on `/login`. `$V api $S GET /api/profiles/me`: `401`, because the API jar shared the same server session. Run `$V login $S` again before `down` so cleanup can reach the PDS.

## Evidence

- `login-anon.png`, `anon-digest.json` + `.status`, `dev-login.status` (on anonymous runs)
- `digest-signed-in.png`, `account-menu.png`
- `after-logout.status`

## Gotchas

- `Log me in` only renders in the Vite dev build and when `GET /dev/login` answers `{"enabled":true}`; `up` sets `DEV_LOGIN_ENABLED` only when doctor found the dev account.
- The server holds one dev session in memory. Browser and `$V login` share it, so logging out in either signs both out, and restarting the instance drops it.
- Console noise on every page: the Vite HMR websocket through the auth gate fails with 302, and `/dev/login` is 404 on anonymous runs. Neither is a failure.
- The OAuth client metadata on an instance still names the `.env` callback (the user's `:8000` server), another reason OAuth is out of scope for runs.
- `Continue` starts real OAuth against the handle's authorization server; agents do not drive it (see Unmapped).
