---
name: verify-morgenblau
description: "Drive the real Morgenblau app like a user and prove a change works: an isolated instance with its own SQLite DB, the React UI in a real browser, the JSON API, inbound newsletter SMTP, sync jobs, evidence in .scratch/verify/, outcome pass, fail or blocked. Also says which static checks (Go, Bun, sqlc, Linux build) a change needs. Use after any user-facing change, before opening a PR, when asked to verify, reproduce or show proof, and when a feature file in features/ matches the work."
---

# Verify Morgenblau

Prove behavior on a running instance, not only in tests. Pick the feature file in [features/](features/README.md) that matches the change, run the protocol below, and report the outcome. Run the static checks for the change type as well; they do not replace the drive.

Every lever is a subcommand of `.agents/skills/verify-morgenblau/bin/verify` (`V` below). `$V help` lists them with their flags.

## Doctor

```sh
V=.agents/skills/verify-morgenblau/bin/verify
$V doctor
```

Run it before the first drive and again after anything surprising. Exit `0`: everything is reachable. Exit `3`: no dev account is configured, so only anonymous surfaces (public pages, auth denial, SMTP ingest, jobs without a session) can be driven; every signed-in step is `blocked` with that reason. Exit `1`: fix what it names first. It prints the env keys it read, never their values.

The dev account is `ATPROTO_HANDLE`, `ATPROTO_PASSWORD` (an app password) and an `https` `ATPROTO_PDS`, in `.env` or the environment. It must be a dedicated development account: runs write real subscription and save records to its PDS. Never ask for or use a personal account.

A sandboxed shell may be unable to bind ports, run `bunx` or read `.env`; rerun the helper outside the sandbox before calling the run `blocked`.

## Levers

| Need | Command |
|---|---|
| Isolated instance: own HTTP, SMTP and Vite ports, fresh migrated DB | `$V up <slug>` (prints state JSON: `base_url`, `smtp`, `dev_login`) |
| Sign in as the dev account (API cookie jar), wait for the login sync, record the PDS baseline | `$V login <slug>` |
| Call the API as that user, or anonymously | `$V api <slug> GET /api/digest --out digest` (`--data JSON`, `--anon`) |
| Read state as JSON | `$V inspect <slug> --out inspect-before` |
| Browser | `$V browser <slug> <playwright-cli command>`; `$V browser <slug> --help` for the driver's commands |
| Deliver a newsletter to the instance | `$V mail <slug> -to <address>` (`-eml FILE` for your own message) |
| Arrange a second reader with a newsletter address | `$V foreign <slug>` |
| Clean up this run | `$V down <slug>` (`--dry-run` first when unsure) |
| Sweep runs left alive by failed attempts | `$V stale` (`--dry-run` lists them) |

The instance runs the working tree with `APP_ENV=local`, `DEV_LOGIN_ENABLED` only when doctor found a dev account, the global feed refresher off (`--fetch-minutes N` turns it on), and newsletters on `newsletter.localhost`. It never touches `./data/morgenblau.db` or a server the user already runs on `:8000`.

In the browser, sign in the way a user does: open `/login` and click `Log me in` (`$V browser <slug> click "getByRole('button', { name: 'Log me in' })"`). Run `$V login <slug>` first anyway: it records the PDS baseline cleanup depends on, and both share the server's single dev session. Target elements by role and accessible name, then label; read the page with `snapshot` or `find`. Use a snapshot ref only for a control without an accessible name, never class names or DOM position.

`$V browser` runs without a prompt, so it passes only the page-level commands in `BROWSER_COMMANDS` (`bin/verify`) and refuses the rest with exit `2` and a one-line reason. `open`, `goto` and `tab-new` take only this run's instance (pass a `/path`). Options that repoint the driver (`--config`, `--profile`, `--browser` and the like), a second session (`-s`) and `run-code` are refused. Every `--filename` and every `upload` file must resolve inside the evidence dir and outside `run/`; relative names resolve from the evidence dir. Page JavaScript goes through `eval`, and waiting for a navigation is a repeated `eval "() => location.href" --raw`. Snapshots, console logs and downloads land in `$E/.playwright-cli/`.

## Protocol

1. `$V stale`, then `$V doctor`.
2. `$V up <slug>`, with the slug named after the feature file. The evidence dir is `.scratch/verify/<YYYY-MM-DD>-<slug>/`; call it `E`.
3. For signed-in features, `$V login <slug>`.
4. `$V inspect <slug> --out inspect-before`.
5. Follow the feature file through its user path. After an action that dispatches a job, wait until `$V api <slug> GET /api/jobs/active` returns `null`, then assert.
6. Capture the evidence the feature file names, then `$V inspect <slug> --out inspect-after` and compare: `diff <(jq -S .counts $E/inspect-before.json) <(jq -S .counts $E/inspect-after.json)`.
7. Run the feature file's negative proofs.
8. `$V down <slug>`. It deletes the PDS records this run added (listed in `E/cleanup.log`), closes the browser session, stops the processes `up` started and removes the run DB. Confirm `E` still holds the evidence.
9. Report.

A failed or abandoned attempt still runs step 8.

## Static checks by change type

`Makefile`, `frontend/package.json` and `.github/workflows/ci.yml` own the commands; read them there. Pick by what changed and report actual results:

- **Go, while working:** the focused package and test (`go test ./internal/<pkg> -run <Test> -count=1`). **Finishing Go work:** the full Go suite and `go vet ./...`; for concurrency changes make `make test-race` the final full run instead.
- **Frontend:** the relevant test files while working; finish with the Bun `test`, `lint` and `build` scripts (the build runs the TypeScript check). `frontend/AGENTS.md` owns the per-branch `doctor` rule.
- **Embedded app or deploy build:** `make build-linux` (it already builds the frontend; do not build twice).
- **Schema or queries:** `make sqlc`, read the regenerated diff, run the affected storage tests against real temporary SQLite. Migrations are tried on a disposable DB (`$V up` gives you one), never on `./data`.
- **Always:** `git diff --check` and a read of the final diff, new files included. Docs-only changes need content, link and format checks instead of app test runs.

## Proof standards

- Drive the real user path. `$V foreign` and direct SQL may arrange preconditions; the behavior under test comes from a browser action, an API call the frontend makes, or an SMTP delivery.
- Capture the action and the resulting state: the screen or response, and the `inspect` delta.
- A step that could not run is not a pass. Proving a feature through a different entry point than the one changed does not count.
- `fail` means the app misbehaved. When a step fails because the feature file is wrong (a renamed button, a changed flow), fix the feature file and drive again. When it fails because the product is wrong, stop driving that feature and report the bug with its evidence; never rewrite the feature file around it.
- `blocked` names its reason: doctor exit 3 for a signed-in step, an unreachable network for PDS or feed calls, a sandbox that cannot bind ports.

## Report

```
verify-morgenblau: pass | fail | blocked
reason: <one line>
features: <feature files driven>
evidence: .scratch/verify/<YYYY-MM-DD>-<slug>/
commands: <levers and drives run>
state delta: <inspect before/after summary>
pds writes: <records created and deleted on the dev account, or none>
static checks: <commands and results>
```

## Safety

- One instance and one DB per run, created by `up`, removed by `down`. The only shared state is the dev account's PDS; `down` deletes what the run added there, and nothing else.
- Never drive the user's own server, database or personal account.
- Newsletter mail goes only to the instance's `127.0.0.1` SMTP port. The app sends no outbound mail and has no payments or admin panel.
- Keep secrets out of evidence: never print or save the dev account password or the session cookie (`run/cookies.txt` is deleted by `down`).
- Never kill processes by name; `down` stops the process ids `up` recorded.
- Cleanup never deletes evidence. Evidence never leaves the machine.
