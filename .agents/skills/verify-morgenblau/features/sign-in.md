# Sign in and out

A visitor who is not signed in lands on `/login`, which asks for an Atmosphere handle; `Continue` signs in through OAuth on the handle's authorization server and returns to the digest. In local development with a dev login account (a run's own account under `$V`), the page also offers `Log me in`, which signs in as that account and opens the digest. Signed-in readers log out from the account menu in the app header, on every app page.

Help article: none

## Sub-features

- `login-page` `/login` renders the handle form.
- `anon-redirect` any app page redirects an anonymous visitor to `/login`; any `/api/*` call answers 401.
- `oauth-sign-in` the handle form's `Continue` posts to `/oauth/login`, the authorization server asks for the password and consent, and `/oauth/callback` lands on `/` (`internal/oauth/handler/flow.go`; a run's loopback PDS goes through `internal/oauth/localflow`).
- `dev-login` `Log me in` signs in as the run's account and lands on `/`.
- `logged-in-redirect` a signed-in reader who opens `/login` gets a `302` to `/`.
- `account-menu` the popover shows the profile and an `Account` navigation (`General`, `Import`, `Export`, see `frontend/src/components/account-menu.tsx`).
- `logout` the account menu's `Log out` is a form `POST /oauth/logout` that ends the session and lands on `/login`.

## How to reach it

- Browser: `/login`, or any app path while signed out.
- `Development login` panel on `/login`, button `Log me in` (only when the run has its PDS).
- App header (`frontend/src/components/app-header.tsx`) on `/`, `/sources`, `/library` and `/settings/*`: button `Open account menu`, then `Log out`.
- API: `GET /dev/login` reports availability, `POST /dev/login` signs in (the `$V login` lever). `$V oauth` drives `POST /oauth/login` through the callback the way a browser does.

## Drive and prove

Preconditions:

- Instance up. `oauth-sign-in`, `dev-login`, `logged-in-redirect` and `logout` need doctor exit `0`. The run's handle is `pds.handle` in `$V up`'s output.

- **Login page.** `$V browser $S open /login`, then `snapshot`. Heading `Sign in with your Atmosphere account`, textbox `Handle`, button `Continue`.
- **Anonymous redirect.** `$V browser $S goto /settings`, then `eval "() => location.href"`: ends on `/login`. `$V api $S GET /api/digest --anon --out anon-digest`: status `401`.
- **Dev login unavailable.** On a doctor-exit-3 run, `$V api $S GET /dev/login --anon --out dev-login`: `404`, and `find "Log me in"` on `/login` finds nothing.
- **OAuth sign-in.** On `/login`, `fill "getByRole('textbox', { name: 'Handle' })" "<handle>"`, click `Continue`, and poll `eval "() => location.href"` until it is the PDS's `/oauth/authorize`. `$V browser $S fill-password "getByRole('textbox', { name: 'Password' })"`, click `Sign in`, wait for `snapshot` to show the `Authorize` button (`screenshot --filename=oauth-consent.png`), click it. Poll until the URL is the instance root; `snapshot` shows `Digest date` and `Open account menu`; `screenshot --filename=oauth-digest.png`. `$V inspect $S` counts one more `oauth_sessions` row.
- **OAuth into the API jar.** `$V oauth $S`, then `$V api $S GET /api/profiles/me --out profile-me` (`200`, the `.test` handle, `needsReauth` false) and `GET /api/digest --out digest` (`200`).
- **Dev login.** On `/login`, `find "Development login"` shows the panel; `click "getByRole('button', { name: 'Log me in' })"`. Poll `eval "() => location.href"` until it is the instance root. `snapshot` shows the date navigation `Digest date` and the button `Open account menu`.
- **Signed-in redirect.** `goto /login`: ends on `/`. Over HTTP, `curl -s -o /dev/null -w '%{http_code} %{redirect_url}' -b $E/run/cookies.txt <base_url>/login` prints `302 <base_url>/`.
- **Account menu.** `click "getByRole('button', { name: 'Open account menu' })"`, `snapshot`: the popover shows `@<handle>`, the `Account` navigation and `Log out`. `click "getByRole('link', { name: 'Import' })"` lands on `/settings/import`.
- **Logout.** From another app page (`goto /sources`), open the account menu and click `Log out`: ends on `/login`. `goto /`: ends on `/login`. After dev login, `$V api $S GET /api/profiles/me --out after-logout` is `401`, because the API jar shared the server's one dev session. After OAuth, the browser and `$V oauth` hold separate sessions: logout drops one `oauth_sessions` row and the API jar still gets `200`.

## Evidence

- `login-anon.png`, `anon-digest.json` + `.status`, `dev-login.status` (on anonymous runs)
- `oauth-consent.png`, `oauth-digest.png`, `profile-me.json`, `digest.json`
- `digest-signed-in.png`, `account-menu.png`
- `after-logout.status`

## Gotchas

- `Log me in` only renders in the Vite dev build and when `GET /dev/login` answers `{"enabled":true}`; `up` sets `DEV_LOGIN_ENABLED` only when it started the run's PDS (doctor exit `0`).
- The server holds one dev session in memory. Browser and `$V login` share it, so logging out in either signs both out, and restarting the instance drops it. OAuth sessions live in the run DB, one per sign-in.
- Console noise on every page: the Vite HMR websocket through the auth gate fails with 302, and `/dev/login` is 404 on anonymous runs. Neither is a failure.
- Runs sign in as the loopback public client (`internal/oauth/config/config.go`), whose callback follows the instance's `PORT`; the confidential client and its metadata document are not exercised.
- The run's PDS cannot resolve the `include:blue.morgen.access` permission set, so `up` asks for the permissions it lists (`local_oauth_scope` in `bin/verify`). The consent screen therefore shows those repo permissions, not the permission set's title and detail.
- `Continue` with any handle but the run's starts OAuth on that account's real server. Drive only the run's handle.
- The authorization server always asks for consent from a loopback client, so every OAuth sign-in passes the `Authorize` screen.
