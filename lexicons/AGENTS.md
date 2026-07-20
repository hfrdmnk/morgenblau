---
paths:
  - "lexicons/**"
---

# Lexicon conventions

- Morgenblau owns `blue.morgen.*`. External lexicons (Bluesky, site.standard.*, margin.at, Glean, Skyreader) are read-only interop; never author or modify them here.
- SPEC.md's `<lexicons>` section is the source of truth. The JSON files under `lexicons/` mirror it; keep both in sync in the same change.
- Schema changes bump `revision` monotonically. Breaking changes (changed types, flipped `required`) are only acceptable while adoption is zero; otherwise evolve additively (optional fields, open unions).
- Publishing to the network is a separate, user-driven step via `goat` as the `morgen.blue` authority account (`did:plc:h7bhafnu5c2p63swrc64zh2z` on eurosky.social): `goat record update --rkey <full-NSID>` with rkey equal to the record's `id`. Never publish as a personal account, and never auto-publish from a coding session; surface the need and let the user run the login.
- Runtime Go code references lexicons through `internal/lexicon` NSID constants and validates records against these schemas before PDS writes.
