# Private newsletters

Each reader gets one private email address on the newsletter domain, made of memorable words (`internal/newsletter/address.go`). Mail sent to it is received over SMTP, stored privately (never on the PDS), grouped into sources by sender, and shown in the digest, the Sources page and the reader. Remote images stay blocked until the reader allows them for a message. Stopping a source deletes its unsaved messages (SPEC.md, Private Newsletter Data).

Help article: none

## Sub-features

- `address` `GET /api/newsletters/address` returns `{}` until an address exists; `POST` creates and returns `{address}`; a second `POST` returns the same address. `GET /api/profiles/me`, which the app shell calls on every page load, creates the address too.
- `smtp-receive` mail to the address is accepted; mail to an unknown local part is refused with `550 5.1.1`.
- `sources` `GET /api/newsletters` returns `{active, stopped}` sources; `GET /api/newsletters/{id}` one source; `GET /api/newsletters/{id}/entries` its messages.
- `source-edit` `PATCH /api/newsletters/{id}` with `{title, primary, tags}`.
- `stop-enable` `POST /api/newsletters/{id}/stop` moves a source to `stopped` and deletes its unsaved messages; `/enable` moves it back and accepts only mail that arrives afterwards.
- `remote-images` a message with remote images starts blocked; `POST /api/newsletters/messages/{id}/images` allows them for that message.
- `move` `POST /api/newsletters/messages/{id}/move` with exactly one of `sourceId` or `newSourceTitle`.
- `save` `POST /api/newsletter-saves` with `{messageId}` saves privately (`201 {id}`); `DELETE /api/newsletter-saves/{id}` removes it; `GET /api/saves` lists it.
- `ui` the address in the `Add a source` dialog ([subscriptions](./subscriptions.md)) and on `/settings/general` (`frontend/src/components/newsletter-address.tsx`); sources on `/sources`; messages in the [entry reader](./entry-reader.md), including `Load images`.
- `unavailable` without a newsletter domain the address routes answer `503` (`writeNewsletterError` in `internal/api/newsletters.go`); every instance has a domain, so this is code-level only.

## How to reach it

- API routes above.
- SMTP: `$V mail $S -to <address>` sends the built-in MIME fixture (HTML, one inline CID image, one remote image) to the instance's receiver; `-eml FILE` sends your own message.
- Browser: `/sources`, `/settings/general`, the `Add a source` dialog, `/entry/<entrySlug>`.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`. `$V inspect $S --out inspect-before`. Do not open the browser or call `/api/profiles/me` before the Address step, or the address already exists.

- **Address.** `$V api $S GET /api/newsletters/address --out address-before`: `{}`. `$V api $S POST /api/newsletters/address --out address`: `{"address":"<word>-<word>-<word>@newsletter.localhost"}`. POST again: same address.
- **Receive.** `$V mail $S -to <address>`: exits 0. Wait about 5 seconds (the processor ticks every 5s), then `$V inspect $S --out inspect-received`: one `newsletter_messages` row for the dev account's DID with `has_blocked_remote_images` 1 and `remote_images_allowed` 0, and no `newsletter_receipts` with a `last_error`.
- **Refused recipient.** `$V mail $S -to nobody@newsletter.localhost`: fails with `550 "5.1.1 recipient not found"`.
- **Sources.** `$V api $S GET /api/newsletters --out sources`: one `active` source titled `sample@sender.example` with `issueCount` 1. `GET /api/newsletters/<id>/entries --out source-entries`: the message.
- **Edit.** `PATCH /api/newsletters/<id> --data '{"title":"Example Letters","primary":true,"tags":["example"]}' --out patch`: the new title, `primary` and tags.
- **Remote images.** `POST /api/newsletters/messages/<messageId>/images --out images`: `remoteImagesAllowed` true; `inspect` shows `remote_images_allowed` 1.
- **Move.** `POST /api/newsletters/messages/<messageId>/move --data '{"newSourceTitle":"Example Moved"}' --out move`: a new source `Example Moved` with `issueCount` 1 holds the message. `--data '{}'` answers `400`.
- **Save.** `POST /api/newsletter-saves --data '{"messageId":"<messageId>"}' --out save`: `201`. `GET /api/saves` lists it with `kind` `newsletter`; `inspect` shows a `newsletter_saves` row and no `user_saves` row (the save never reaches the PDS).
- **Stop keeps saved messages.** `POST /api/newsletters/<Example Moved id>/stop --out stop`: `status` `stopped`; `GET /api/newsletters --out sources-stopped` lists it under `stopped`; the saved message is still in `inspect`. `POST .../enable --out enable`: back in `active`.
- **UI.** Sign the browser in, `goto /settings/general`: heading `General settings`, textbox `Newsletter email address` holding the address; `screenshot --filename=settings-general.png`. `goto /sources`: region `Newsletters` lists `Example Letters` with `Primary` and the sender `sample@sender.example`; `screenshot --filename=sources-newsletters.png --full-page`.
- **Unsave.** `DELETE /api/newsletter-saves/<id>`: `204`.
- **Digest.** See [digest](./digest.md): the message appears in `/` for today.
- **Negative.** `GET /api/newsletters/address --anon`: `401`. See [tenant-isolation](./tenant-isolation.md) for another reader's messages.

## Evidence

- `address-before.json`, `address.json`, `mail.log`
- `inspect-before.json`, `inspect-received.json`, `inspect-after.json`
- `sources.json`, `source-entries.json`, `patch.json`, `images.json`, `move.json`, `save.json`, `stop.json`, `sources-stopped.json`, `enable.json`
- `settings-general.png`, `sources-newsletters.png`

## Gotchas

- Delivery is asynchronous: the SMTP session stores a receipt, the processor turns it into a message within about 5 seconds. Asserting sooner reports a false fail.
- Stopping a source before the image, move and save steps deletes the message they need; redeliver after `enable` if that happens.
- The address domain is always `newsletter.localhost` on a verify instance; the `.env` newsletter settings are overridden.
- Newsletter data lives only in the run DB, so `down` removes it with the DB; nothing to clean on the PDS.
