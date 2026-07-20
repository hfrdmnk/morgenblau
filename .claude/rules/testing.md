# Testing conventions

- **Generic fixtures only.** Test data never references real people, handles, domains, or publication names, even ones encountered while researching a feature (no `realuser.leaflet.pub`, no real DIDs beyond syntactically valid placeholders). Use reserved namespaces: `example.com` subdomains for hosts, `*.example` for handles, invented names like "Example Publication" for record fields. Real-world observations belong in research notes or SPEC.md, not in fixtures.
