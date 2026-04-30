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
