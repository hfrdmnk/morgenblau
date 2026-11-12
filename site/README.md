# Morgenblau site

The landing page for `morgen.blue`, built with [Astro](https://docs.astro.build).

```sh
bun install
bun run dev
```

`bun run build` writes static files to `dist/`, which the `morgenblau-site` Cloudflare Worker serves as static assets (`wrangler.toml`). The build fetches the digest the hero's app screen shows: Dominik's subscriptions from his PDS and their posts for the current day in Zurich (`src/lib/hero-digest.ts`).

Workers Builds builds and deploys the Worker from this directory on every push to `main` that touches `site/`. To keep the hero current, the Worker's hourly cron trigger (`worker.js`) also POSTs a Workers Builds deploy hook for `main`, whose URL is in the Worker's `DEPLOY_HOOK_URL` secret.
