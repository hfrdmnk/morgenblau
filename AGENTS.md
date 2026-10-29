# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

## Layout

This is a monorepo. Each project builds and deploys on its own; nothing is shared between them yet.

- [`app/`](app/AGENTS.md): the product, a Go backend and React frontend. Deploys to Fly.
- `site/`: the Astro landing page for `morgen.blue`. Deploys to Fly.
- `brand/`: Abstract Studio, the Vite/React tool that generates brand artwork. Deploys to Vercel.

Bun owns every JavaScript install; each project keeps its own `bun.lock`.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec. [BRAND.md](./BRAND.md) owns the brand direction.

## Comments

Comment the why, never the what. A comment earns its place only when it records something the code cannot express: a non-obvious constraint, an ordering requirement, a protocol quirk, a deliberate trade-off. Never restate the next line, narrate a change, or justify a diff. If code needs explaining, rewrite the code first. One line by default. Applies to all languages in this repo.

## Docs

Docs point, they don't mirror. In durable project docs (the AGENTS.md files, `SPEC.md`, `.claude/rules/*`, memory), write pointers and invariants, not a copy of what the code already owns. Name the file or function that holds the detail and state the rule; don't enumerate a list the code will grow past (an error-code set, a route table).

## Testing

Review identity fixtures: use `example.com` subdomains for ordinary hosts, `*.example` handles, names like "Example Publication", and placeholder DIDs. Real protocol/provider hostnames are allowed only when the hostname itself is required to test behavior; accounts, publications and other identity data remain invented. Real-world observations belong in research notes or `SPEC.md`.

## Git & PRs

- **GitHub is the source of truth, Tangled is a read-only mirror.** Ship to GitHub's `main`; `.github/workflows/sync-tangled.yml` mirrors every GitHub branch to Tangled, deletions included.
- `app/scripts/sync-tangled.sh` is the manual recovery path when the mirror workflow needs to be retried outside GitHub.
- Never bypass repository Git hooks with `--no-verify` unless the user explicitly approves it for that invocation. Investigate and resolve hook failures instead.
