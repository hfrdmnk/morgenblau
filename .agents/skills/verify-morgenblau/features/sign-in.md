# Sign in and out

A visitor who is not signed in lands on `/login`, which asks for an Atmosphere handle. In local development with a configured dev account, the page also offers `Log me in`, which signs in as that account and opens the digest. Signed-in readers log out from the account menu in the app header, on every app page.

Help article: none

## Sub-features

- `login-page` `/login` renders the handle form.
- `anon-redirect` any app page redirects an anonymous visitor to `/login`; any `/api/*` call answers 401.
- `dev-login` `Log me in` signs in as the dev account and lands on `/`.
- `logged-in-redirect` a signed-in reader who opens `/login` gets a `302` to `/`.
- `account-menu` the popover shows the profile and an `Account` navigation (`General`, `Import`, `Export`, see `frontend/src/components/account-menu.tsx`).
- `logout` the account menu's `Log out` is a form `POST /oauth/logout` that ends the session and lands on `/login`.

## How to reach it

- Browser: `/login`, or any app path while signed out.
- `Development login` panel on `/login`, button `Log me in` (only when the dev account is configured).
- App header (`frontend/src/components/app-header.tsx`) on `/`, `/sources`, `/library` and `/settings/*`: button `Open account menu`, then `Log out`.
- API: `GET /dev/login` reports availability, `POST /dev/login` signs in (the `$V login` lever).

## Drive and prove

Preconditions:

- Instance up. `dev-login`, `logged-in-redirect` and `logout` need doctor exit `0`.

- **Login page.** `$V browser $S open /login`, then `snapshot`. Heading `Sign in with your Atmosphere account`, textbox `Handle`, button `Continue`.
- **Anonymous redirect.** `$V browser $S goto /settings`, then `eval "() => location.href"`: ends on `/login`. `$V api $S GET /api/digest --anon --out anon-digest`: status `401`.
- **Dev login unavailable.** On a doctor-exit-3 run, `$V api $S GET /dev/login --anon --out dev-login`: `404`, and `find "Log me in"` on `/login` finds nothing.
- **Dev login.** `$V login $S` first (PDS baseline). Then on `/login`, `find "Development login"` shows the panel; `click "getByRole('button', { name: 'Log me in' })"`. Poll `eval "() => location.href"` until it is the instance root. `snapshot` shows the date navigation `Digest date` and the button `Open account menu`.
- **Signed-in redirect.** `goto /login`: ends on `/`. Over HTTP, `curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -b $E/run/cookies.txt <base_url>/login` prints `302 <base_url>/`.
- **Account menu.** `click "getByRole('button', { name: 'Open account menu' })"`, `snapshot`: the popover shows `@<handle>`, the `Account` navigation and `Log out`. `click "getByRole('link', { name: 'Import' })"` lands on `/settings/import`.
- **Logout.** From another app page (`goto /sources`), open the account menu and click `Log out`: ends on `/login`. `goto /`: ends on `/login`. `$V api $S GET /api/profiles/me --out after-logout`: `401`, because the API jar shared the same server session. Run `$V login $S` again before `down` so cleanup can reach the PDS.

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
