# Lexicon ideas — future work

Notes on lexicon changes we want to land later, kept here so we don't lose the thread.

## Standardising `sourceType` on `app.skyreader.feed.subscription`

The lexicon currently describes `sourceType` as `'rss', 'atproto.shares', 'atproto.documents', 'atproto.collection'. Omitted means RSS.` That mixes the protocol axis (RSS vs ATProto stream) with the catch-all `rss` value.

### Today (interim)

The Morgenblau client writes `sourceType` as one of:

- `rss` — default; generic blog/article RSS
- `video` — YouTube etc.
- `podcast` — audio feeds
- `microblog` — Mastodon / micropost-style streams

These values are written as plain strings against the current schema (`type: "string"`, no enum), so they're already valid; the description is just stale.

### Tomorrow (proposed)

Refine the field to make the value space explicit and make the lexicon useful for any reader in the atmosphere:

```json
"sourceType": {
  "type": "string",
  "knownValues": ["rss", "video", "podcast", "microblog"],
  "maxLength": 64,
  "description": "Content source type. Omitted means 'rss'. Open union — clients should tolerate unknown values."
}
```

Open question: how to express ATProto-stream subscriptions (the original `atproto.shares` / `atproto.documents` / `atproto.collection` direction):

- **Option A — keep them under `sourceType`.** The open union accommodates both axes (content type + protocol). Simplest; matches what's shipping today.
- **Option B — introduce a separate `protocol` field** (e.g. `rss` | `atproto`) and free `sourceType` purely for content classification. Cleaner separation, more fields.

Migration path: clients that read `sourceType` should fall back to `'rss'` when absent or unknown. No mass migration required — values fill in over time.

Goal: a generally-useful RSS-subscription lexicon that any reader can adopt.

## Conditional-required fields on `app.skyreader.feed.subscription`

The current schema requires only `createdAt`. `feedUrl` and `subjectDid` are both optional. In practice every record needs **one or the other** depending on `sourceType`:

- For URL-driven types (`rss`, `video`, `podcast`, `microblog`): `feedUrl` is required.
- For ATProto-stream types (`atproto.shares`, `atproto.documents`, `atproto.collection`): `subjectDid` is required and `feedUrl` is meaningless.

The lexicon spec doesn't currently express conditional-required fields. Options:

- **Today (interim):** enforce client-side. The Morgenblau client validates inputs before writing; consumers should reject records missing the field implied by their `sourceType`. The lexicon doesn't help here, so reader/writer correctness is by convention.
- **Proposal A — split the record into two collections.** `app.skyreader.feed.subscription` for URL feeds (`feedUrl` required), `app.skyreader.account.subscription` for ATProto streams (`subjectDid` required). Cleaner; doubles the surface area; breaks the "one collection per concept" framing currently shared with Skyreader.
- **Proposal B — `oneOf` constraint on the lexicon.** Add a top-level `oneOf: [{required: ["feedUrl"]}, {required: ["subjectDid"]}]`. Requires an upstream change to the lexicon spec — `oneOf` isn't part of the current grammar. Out of our hands without coordination.

**Decision:** defer. Document for now. Revisit when (a) an ATProto-stream `sourceType` actually ships, or (b) the lexicon spec gains `oneOf`/`if/then` semantics.
