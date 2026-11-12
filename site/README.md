# Morgenblau site

The landing page for `morgen.blue`, built with [Astro](https://docs.astro.build).

```sh
bun install
bun run dev
```

`bun run build` writes static files to `dist/`; the `Dockerfile` serves them with nginx. The build fetches the digest the hero's app screen shows: Dominik's subscriptions from his PDS and their posts for the current day in Zurich (`src/lib/hero-digest.ts`). `.github/workflows/site-refresh.yml` rebuilds and redeploys every hour to keep it current. Pushes to `main` deploy to Fly from the `deploy` job in `.github/workflows/ci.yml` once CI passes, using the `FLY_API_TOKEN_SITE` secret.
