# Private newsletters

Each reader can create one private email address on the newsletter domain. Mail sent to it is received over SMTP, stored privately (never on the PDS), grouped into sources by sender, and shown in the digest. Remote images stay blocked until the reader allows them for a message. The Newsletters screen is not built yet, so every step below goes through the API the frontend will use.

Help article: none

## Sub-features

- `address` `GET /api/newsletters/address` returns `{}` before creation; `POST` creates and returns `{address}`; a second `POST` returns the same address.
- `smtp-receive` mail to the address is accepted; mail to an unknown local part is refused with `550 5.1.1`.
- `sources` `GET /api/newsletters` returns `{active, stopped}` sources; `GET /api/newsletters/{id}` one source; `GET /api/newsletters/{id}/entries` its messages.
- `source-edit` `PATCH /api/newsletters/{id}` with `{title, primary, tags}`.
- `stop-enable` `POST /api/newsletters/{id}/stop` and `/enable` move a source between `stopped` and `active`.
- `remote-images` a message with remote images starts blocked; `POST /api/newsletters/messages/{id}/images` allows them for that message.
- `move` `POST /api/newsletters/messages/{id}/move` with exactly one of `sourceId` or `newSourceTitle`.
- `save` `POST /api/newsletter-saves` with `{messageId}` saves privately (`201 {id}`); `DELETE /api/newsletter-saves/{id}` removes it; `GET /api/saves` lists it.
- `digest` a received message appears in the digest for its day.

## How to reach it

- API routes above.
- SMTP: `$V mail $S -to <address>` sends the built-in MIME fixture (HTML, one inline CID image, one remote image) to the instance's receiver; `-eml FILE` sends your own message.

## Drive and prove

Preconditions:

- Doctor exit `0`, `$V login $S`. `$V inspect $S --out inspect-before`.

- **Address.** `$V api $S GET /api/newsletters/address --out address-before`: `{}`. `$V api $S POST /api/newsletters/address --out address`: `{"address":"<local>@newsletter.localhost"}`. POST again: same address.
- **Receive.** `$V mail $S -to <address>`: exits 0. Wait about 5 seconds (the processor ticks every 5s), then `$V inspect $S --out inspect-received`: one `newsletter_messages` row for the dev account's DID with `has_blocked_remote_images` 1 and `remote_images_allowed` 0, and no pending `newsletter_receipts`.
- **Refused recipient.** `$V mail $S -to nobody@newsletter.localhost`: fails with `550 "5.1.1 recipient not found"`.
- **Sources.** `$V api $S GET /api/newsletters --out sources`: one `active` source titled `sample@sender.example` with `issueCount` 1. `GET /api/newsletters/<id>/entries --out source-entries`: the message, `hasBlockedRemoteImages` true.
- **Edit, stop, enable.** `PATCH /api/newsletters/<id> --data '{"title":"Example Letters","primary":true,"tags":["example"]}'`, then `POST .../stop` (source in `stopped`), then `POST .../enable` (back in `active`). Read `GET /api/newsletters` after each.
- **Remote images.** `POST /api/newsletters/messages/<messageId>/images --out images`: `remoteImagesAllowed` true; `inspect` shows `remote_images_allowed` 1.
- **Move.** `POST /api/newsletters/messages/<messageId>/move --data '{"newSourceTitle":"Example Moved"}' --out move`: a new source holds the message. `--data '{}'` answers `400`.
- **Save.** `POST /api/newsletter-saves --data '{"messageId":"<messageId>"}' --out save`: `201`. `GET /api/saves` lists it; `inspect` shows a `newsletter_saves` row and no `user_saves` row (the save never reaches the PDS). `DELETE /api/newsletter-saves/<id>`: `204`.
- **Digest.** See [digest](./digest.md): the message appears in `/` for today.
- **Negative.** See [tenant-isolation](./tenant-isolation.md) for another reader's messages.

## Evidence

- `address-before.json`, `address.json`, `mail.log`
- `inspect-before.json`, `inspect-received.json`, `inspect-after.json`
- `sources.json`, `source-entries.json`, `images.json`, `move.json`, `save.json`

## Gotchas

- Delivery is asynchronous: the SMTP session stores a receipt, the processor turns it into a message within about 5 seconds. Asserting sooner reports a false fail.
- The address domain is always `newsletter.localhost` on a verify instance; the `.env` newsletter settings are overridden.
- Newsletter data lives only in the run DB, so `down` removes it with the DB; nothing to clean on the PDS.
