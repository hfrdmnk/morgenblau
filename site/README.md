# Morgenblau site

The landing page for `morgen.blue`, built with [Astro](https://docs.astro.build).

```sh
bun install
bun run dev
```

`bun run build` writes static files to `dist/`; the `Dockerfile` serves them with nginx. Pushes to `main` deploy to Fly through Fly's GitHub integration, with its working directory set to `site`.
