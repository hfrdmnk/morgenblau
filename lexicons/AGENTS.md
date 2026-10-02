---
paths:
  - "lexicons/**"
---

# Lexicon conventions

- Morgenblau owns `blue.morgen.*`. Every other lexicon here is read-only interop; never author or modify one.
- The JSON files under `lexicons/` are the schema authority; `TestEmbeddedSchemasMatchTheLexiconsDirectory` keeps the validator's embedded copies identical. SPEC.md's `<lexicons>` section defines compatibility constraints without duplicating schemas. Keep existing schemas unchanged during the v1 simplification.
- Schema changes bump `revision` monotonically. Breaking changes (changed types, flipped `required`) are only acceptable while adoption is zero; otherwise evolve additively (optional fields, open unions).
- Publishing to the network is a separate, user-driven step via `goat` as the `morgen.blue` authority account (`did:plc:h7bhafnu5c2p63swrc64zh2z` on eurosky.social): `goat record update --rkey <full-NSID>` with rkey equal to the record's `id`. Never publish as a personal account, and never auto-publish from a coding session; surface the need and let the user run the login.
- Go code names `blue.morgen` NSIDs only through `internal/lexicon` constants (`TestBlueMorgenNSIDsComeFromLexiconConstants`) and validates records with `lexicon.ValidateRecord` before PDS writes.
