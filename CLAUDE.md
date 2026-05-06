# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

## Stack

Laravel 13 (PHP 8.4) + Inertia 3 + React 19 + TypeScript + Tailwind v4. Auth via ATProto OAuth using `revolution/laravel-bluesky`. Pest for backend tests. Frontend routing via Laravel Wayfinder (`@/routes`, `@/actions`).

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec. When you notice a discrepancy between the code and the spec, flag it and either ask to update the spec or adapt the code — never silently diverge.

## Lexicons

The `lexicons/` directory is reference, with the goal of evolving a standardised RSS-reader lexicon for the AT atmosphere. Lexicon `.json` files are read-only from this codebase — Morgenblau adheres to whatever they say. The only file in `lexicons/` you may edit is `IDEAS.md`.

When you spot an opportunity to improve the lexicon (new field, refined semantics, new `knownValues`, etc.), surface it as a question to the user. On approval, record the proposal in `lexicons/app/skyreader/IDEAS.md` under a clearly-labelled section. Don't silently change the JSON.

## Git & PRs

- **Tangled is the source of truth, GitHub is a mirror.** `origin` points at `git@tangled.org:dominik.social/morgenblau`; `git push` auto-mirrors to GitHub via a second pushurl. Use `bun run sync:github` to repair the mirror if it drifts.
- **Open PRs on tangled, not GitHub.** Don't run `gh pr create`.

## Tooling

- **Package manager: bun only.** Never use npm, yarn, or pnpm for dependency changes. All install/run/build commands must use `bun`. PHP deps via Composer.
- **Never use inline eval** (`node -e`, `bun -e`, or equivalent). Don't create temporary files to work around this. To investigate JS package APIs, use [npmx.dev](https://npmx.dev) instead.
- **Wayfinder over hardcoded URLs.** Import route helpers from `@/routes/*` and action helpers from `@/actions/*`. Re-run `php artisan wayfinder:generate` after route or controller changes.
- **Inertia + React pages live at `resources/js/pages/`.** Kebab-case filenames, default export a component.
- **Data shapes use `spatie/laravel-data`.** TS types are generated via `spatie/laravel-typescript-transformer` (`php artisan typescript:transform`, output: `resources/js/types/generated.d.ts`). Don't hand-roll TS interfaces that mirror PHP shapes — see `.claude/rules/laravel-data.md`.

## Testing

- Use **Red/Green TDD** as the standard development paradigm.
- **Pest** for backend tests (`tests/Feature`, `tests/Unit`). Run with `php artisan test --compact` — add `--filter=<name>` to scope.

## Special Skills

- For any frontend/UI task (components, styling, layout, copy, polish), invoke `morgenblau-designer`.
- For ATProto protocol work, invoke the matching skill: `atproto-oauth`, `atproto-lexicon`, `atproto-publish-lexicon`, `atproto-identity-resolution`, `atproto-repository`, `atproto-cid`, `atproto-attestation`.

## Verification

After each batch of work, run:

- `bun run lint` — ESLint (0 errors, 0 warnings)
- `bun run types` — TypeScript check
- `./vendor/bin/pint --dirty --format agent` — PHP formatter (Laravel's style)
- `php artisan test --compact` — Pest suite

## Key References

- [SPEC.md](./SPEC.md): Product spec
- [atproto.com](https://atproto.com/): General ATproto docs
- [revolution/laravel-bluesky docs](https://github.com/invokable/laravel-bluesky/blob/main/docs/socialite.md)
- Skyreader lexicons: `lexicons/app/skyreader/`
- [Microcosm](https://www.microcosm.blue/): Helpful APIs (e.g. for backlink discovery on the protocol)
