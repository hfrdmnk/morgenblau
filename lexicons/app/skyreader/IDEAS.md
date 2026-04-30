# Lexicon ideas — future work

Notes on lexicon changes we want to land later, kept here so we don't lose the thread.

## Feed-level content type on `app.skyreader.feed.subscription`

We need a way to say "this source is a video / podcast / micropost / blog" at the source level so digest assembly and the consume UI can group and filter consistently.

### Today (interim)

The type is encoded as a prefix in the existing `category` string:

- `source:video`
- `source:podcast`
- `source:micropost`
- `source:blog`

The Morgenblau adapters set this when the user adds a source. Records that don't carry the prefix (e.g. ones written by Skyreader or another client) are treated as type-unknown — we fall back to `blog` for layout and avoid type-aware grouping until the user re-saves the record from Morgenblau.

### Tomorrow

Add a dedicated optional field on `app.skyreader.feed.subscription`:

```json
"feedType": {
  "type": "string",
  "knownValues": ["blog", "video", "podcast", "micropost"]
}
```

Open union — an unrecognized value should be tolerated by older clients without breaking. Reading order:

1. `feedType` if present, trust it.
2. Else parse the `source:<type>` prefix from `category`.
3. Else infer from `feedUrl` host (youtube.com → video, etc.) as a last resort.

Migration path: when a user edits a sub that lacks `feedType`, backfill on the next write. No mass migration required — the field just fills in over time.
