# Morgenblau

A personal daily newspaper powered by RSS and ATProto. [SPEC.md](SPEC.md) defines the product; [BRAND.md](BRAND.md) the brand.

| Folder | What | Stack | Hosting |
| --- | --- | --- | --- |
| [`app/`](app/README.md) | The product at `app.morgen.blue` | Go + React/Vite | Fly (`morgenblau`), via Fly's GitHub integration with working directory `app` |
| [`site/`](site/README.md) | Landing page at `morgen.blue` | Astro | Fly (`morgenblau-site`), via Fly's GitHub integration with working directory `site` |
| [`brand/`](brand/README.md) | Abstract Studio, the brand artwork generator | Vite/React | Vercel, with Root Directory `brand` |

Each folder is its own project with its own `bun.lock`; run commands from inside it. Bun is the only package manager.

## Git & PRs

[GitHub](https://github.com/hfrdmnk/morgenblau) is the source of truth; [Tangled](https://tangled.org/dominikhofer.me/morgenblau) is a read-only mirror.

Every branch push and deletion on GitHub automatically syncs to Tangled through `.github/workflows/sync-tangled.yml`. Run `app/scripts/sync-tangled.sh` to retry the mirror manually.

Activate the committed Git hooks once per clone with `make -C app hooks`.
