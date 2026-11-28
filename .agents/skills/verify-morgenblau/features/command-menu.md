# Command menu

`⌘K` (or `Ctrl+K`) opens a keyboard palette on every signed-in page. It opens on one unheaded list of actions, the page's own first, then `Go to`; sources, saves and the day's headlines appear only once the reader types. A command runs at once, flips a toggle in place, or takes over the input with a step (text, choice, tags or confirm). Every write goes through the same API the page's own controls use.

Help article: none

## Sub-features

- `open` `CommandMenu` in `frontend/src/components/command-menu.tsx`, mounted outside `AppShell` so it works on the reader too. `isCommandMenuKey` (`frontend/src/lib/shortcuts.ts`) accepts Meta or Ctrl with `k`. Disabled on `/login`. Another open dialog, such as the `Add a source` sheet, keeps the keys, so the menu never stacks on it. It is a dialog titled `Command menu` with the combobox `Search commands` (placeholder `Type a command, source or day`) focused. Closing it returns focus to whatever had focus before it opened.
- `ranking` `rankedGroups`: with no query, `ACTIONS` (no visible heading, accessible name `Actions`), then `Go to`, then the search groups; with one, the best match leads. `searchOnly` rows (`Sources`, `Library`, the digest's headlines under the day's name) list only while typing. Matching reads the label, keywords and detail, never the id, and ignores case and accents. Nothing matching reads `Nothing matches “<query>”.`
- `query rows`: a typed day such as `14.11` leads with `Go to <day>`. A query that parses as a site leads with `Add <site> as a source`, which opens the `Add a source` sheet with that URL.
- `global commands` `useGlobalCommands`. `Go to`: the app modes, with `Library` reading `Recent` and a second `Library` reading `Everything`, `Day…` (a choice of the last 28 days), `General settings`, `Import sources`, `Export sources`. `Actions`: `Add a source…`, `Rename a source…` and `Delete a source…` (a choice of every feed and newsletter that leads to the rename or delete step; a newsletter is deleted, not stopped), `Refresh sources` (`POST /api/digest/refresh`, toast `Refreshing your sources`), `Reader font` (choice, kept in localStorage `morgenblau-reader-font`), `Copy newsletter address` (toast `Newsletter address copied`), `Download OPML export` (`POST /api/subscriptions/export`, toast `Your OPML download is ready.`), the `Public following list` toggle (`/api/public-following`), `Keyboard shortcuts` (a choice listing `SHORTCUTS`) and `Log out`.
- `page commands` are registered with `useCommands` (`frontend/src/lib/commands.ts`). Pages register into `ACTIONS`, ahead of the global actions. Digest (`frontend/src/pages/digest.tsx`): `Previous day`, and `Next day` and `Today` on a past day, plus the headlines in a group named after the weekday. Library Everything (`useSortCommand` in `frontend/src/pages/library.tsx`): `Sort by…`. Feed source page (`useFeedSourceCommands` in `frontend/src/hooks/use-source-commands.ts`): `Rename <name>…`, `Edit tags`, `Primary`, `Include Shorts` for YouTube, `Open site`, `Copy feed URL`, `Open latest post` and `Delete <name>…`. Newsletter source page: `useNewsletterCommands`, with `Rename <name>…` and `Delete <name>…`. Reader (`frontend/src/pages/entry.tsx`): `Saved to Library`, `Edit tags`, `Read original`, `Copy link to original`, `Go to <source>`, `Previous entry`, `Next entry` and `Back to digest`.
- `steps` (`StepView`): the input's accessible name becomes the step's title (`Rename`, `Go to day`, `Reader font`, `Delete source`). Escape inside a step returns to the step it came from (`parent`), or to the top level; Escape at the top closes the menu. A confirm step reads `Press Enter to confirm, Esc to go back`.
- `results` `useRun`: a command that resolves to a message closes the menu and toasts it; a toggle stays open and shows its new state. A failure stays in the menu as an alert: a `CommandProblem` message as is, anything else `That didn’t work. Try again.`

## How to reach it

- Browser: `press Control+k` on any signed-in page. `type` fills the focused search input; `press Enter` runs the selected row, and `ArrowDown` moves the selection.
- No API of its own; each command calls the routes its page uses.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`. Subscribe a fixture feed with a title through the API, so it has a name to search: `$V api $S POST /api/subscriptions --data '{"feedUrl":"<fixtures.url>/feeds/valid.xml","title":"Morning Fixture","siteUrl":"<fixtures.url>/"}'`, then wait until `GET /api/jobs/active` is `null`.
- Read the menu's state with `eval`: the dialog's `[cmdk-group-heading]` and `[cmdk-item]` texts, the selected row's `aria-selected`, the input's `aria-label` and value, and any `[role=alert]` text.

Steps:

- **Disabled on login.** On `/login`, `press Control+k` and `press Meta+k`: no dialog opens.
- **Open.** Sign in with `Log me in`. On `/`, `press Control+k`: dialog `Command menu`, `Search commands` focused, an unheaded list led by `Previous day` and `Add a source…`, then `Go to`, and no source, save or headline rows (`command-menu-open.png`). The list scrolls, so `Actions` can sit below the fold in the screenshot.
- **Search.** Type `fixture entry`: the `<weekday>` group lists `Fixture entry published just now / Morning Fixture`, and Enter opens `/entry/<slug>?from=<date>`. Type `morning`: `Sources` lists `Morning Fixture`, and Enter opens `/sources/<rkey>`. With the latin1 entry saved, type `zurich`: `Library` lists `Grüße aus Zürich, café crème`, and Enter opens `/entry/<slug>?from=/library?view=everything`. A query nothing matches reads `Nothing matches “<query>”.`
- **Page actions.** On `/sources/<rkey>`, the list opens on `Rename Morning Fixture…` … `Delete Morning Fixture…`, ahead of `Add a source…` (`command-menu-source-page.png`). On the entry it opens on `Saved to Library` (`command-menu-reader.png`).
- **Pick a source.** On `/`, `Delete a source…` lists every source; Enter on one opens `Delete source`, Escape returns to `Delete a source`, and Enter on the confirm deletes it: toast `Source deleted`, the page stays on `/`, and `GET /api/subscriptions` no longer lists it.
- **Rename.** Enter on `Rename Morning Fixture…`: the input reads `Rename` with the current name, and the row reads `Rename to “<value>”`. Select all, type a new name, Enter: toast `Source renamed`, and `GET /api/subscriptions` shows the new `title` in both the record `value` and the top level.
- **Toggles.** `Primary`: the row flips to `On` in place, and the API shows `primary: true`. `Public following list`: `GET /api/public-following` reads `enabled: true` and `/user/<did>/following.opml` answers `200`; flip it back for `false` and `404`. On the entry, `Saved to Library` flips to `On`, and `GET /api/saves` lists the entry.
- **Choices.** `Day…`, type `yesterday`, Enter: `/?date=<yesterday>`. Typing `14.11` at the top level gives `Go to <weekday>, November 14`, and Enter opens `/?date=<year>-11-14`. `Reader font`, `serif`, Enter: localStorage `morgenblau-reader-font` is `serif`, and the row's detail reads `Serif`. `Keyboard shortcuts` lists `Command menu / Anywhere / Ctrl K` first.
- **Actions.** `Refresh sources`: toast `Refreshing your sources`, and `requests` shows `POST /api/digest/refresh` with `200`. `Copy newsletter address`: toast `Newsletter address copied`, after `GET /api/newsletters/address`. `Download OPML export`: toast `Your OPML download is ready.`, and `$E/.playwright-cli/morgenblau.opml` lists the source. `Add a source…` opens the `Add a source` sheet.
- **Escape.** Inside `Day…`, Escape returns to `Search commands` with the menu still open; a second Escape closes it. Focus a header button such as `Add a source`, open the menu and press Escape: the button has focus again.
- **Delete.** On the source page, type `delete morning` and press Enter: the step `Delete source` reads `Its posts leave your digest. Posts you saved stay in your Library.` and `Delete “<current name>”` (`command-menu-delete-confirm.png`). Escape returns with the source kept; Enter again deletes it: the page lands on `/sources`, toast `Source deleted`, and `GET /api/subscriptions` is `[]`.
- **Log out.** `Log out`: the page lands on `/login`, `$V api $S GET /api/subscriptions` answers `401`, and `/` redirects to `/login`. Do this last; the browser and `$V login` share one session.

Negative proofs:

- `/login` opens no menu with either modifier.
- With the `Add a source` sheet open, `press Control+k` leaves exactly one dialog, the sheet. Check both ways in: the header button, and the menu's own `Add a source…` opened from a focused page control (reload `/`, blur, `press Tab` twice to focus a header link, then `Control+k`). Either way `document.activeElement` is inside the sheet and `Control+k` opens nothing (`command-menu-over-sheet.png` when it does).
- On `/`, with no query, no source, save or headline row lists.
- Escape on a confirm step deletes nothing: `GET /api/subscriptions` still lists the source.

## Evidence

- The screenshots above, plus `command-menu-search.png` and `command-menu-rename.png`
- `inspect-before.json`, `inspect-after.json`; the `requests` lines for refresh and export

## Gotchas

- `fill` on `Search commands` can leave the old value in place. Clear it with `press Control+a` and `press Backspace`, then `type`.
- A query that looks like a host (`valid.xml`, `example.com`) leads with `Add <site> as a source`, ahead of the matching source. Search sources by their name.
- A source whose name contains the command you type ranks above that command: with a source named `Delete Probe`, `delete` selects the source. Type the full command label (`delete source`).
- Each `$V browser` call takes about a second, so a check after separate `press` and `eval` calls can miss a toast; toasts stay about 4 seconds. To time them, install a `MutationObserver` on `[data-sonner-toast]` with `eval` before the action.
- `navigator.clipboard.readText` is refused in the driver (`NotAllowedError`). The copy commands toast only after `writeText` resolves, so the toast is the proof.
