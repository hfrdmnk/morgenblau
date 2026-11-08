# Morgenblau site

The landing page for `morgen.blue`, built with [Astro](https://docs.astro.build).

```sh
bun install
bun run dev
```

`bun run build` writes static files to `dist/`; the `Dockerfile` serves them with nginx. Pushes to `main` deploy to Fly from the `deploy` job in `.github/workflows/ci.yml` once CI passes, using the `FLY_API_TOKEN_SITE` secret.
