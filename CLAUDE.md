# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

## Stack

Laravel 13 (PHP 8.4) + Inertia 3 + React 19 + TypeScript + Tailwind v4. Auth via ATProto OAuth using `revolution/laravel-bluesky`. Pest for backend tests. Frontend routing via Laravel Wayfinder (`@/routes`, `@/actions`).

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec. When you notice a discrepancy between the code and the spec, flag it and either ask to update the spec or adapt the code — never silently diverge.

## Tooling

- **Package manager: bun only.** Never use npm, yarn, or pnpm for dependency changes. All install/run/build commands must use `bun`. PHP deps via Composer.
- **Never use inline eval** (`node -e`, `bun -e`, or equivalent). Don't create temporary files to work around this. To investigate JS package APIs, use [npmx.dev](https://npmx.dev) instead.
- **Wayfinder over hardcoded URLs.** Import route helpers from `@/routes/*` and action helpers from `@/actions/*`. Re-run `php artisan wayfinder:generate` after route or controller changes.
- **Inertia + React pages live at `resources/js/pages/`.** Kebab-case filenames, default export a component.

## Testing

- Use **Red/Green TDD** as the standard development paradigm.
- **Pest** for backend tests (`tests/Feature`, `tests/Unit`). Run with `php artisan test --compact` — add `--filter=<name>` to scope.
- Prefer feature tests over unit tests unless the logic is genuinely pure.
- **Extract shared setup into traits** (or Pest `beforeEach`/helpers) instead of repeating arrange code across individual test cases. Reach for a trait the second time you'd copy-paste setup.
- **Test like an experienced senior dev**: focus on meaningful behavior and likely failure modes, not exhaustive edge-case enumeration. Each test should earn its place.
- No frontend test runner is installed yet; add Vitest when the first interaction-worthy UI lands.

## Skills

- For any frontend/UI task (components, styling, layout, copy, polish), invoke `morgenblau-designer`.
- For ATProto protocol work, invoke the matching skill: `atproto-oauth`, `atproto-lexicon`, `atproto-publish-lexicon`, `atproto-identity-resolution`, `atproto-repository`, `atproto-cid`, `atproto-attestation`.

## Verification

After each batch of work, run:

- `bun run lint` — ESLint (0 errors, 0 warnings)
- `bun run types` — TypeScript check
- `./vendor/bin/pint --dirty --format agent` — PHP formatter (Laravel's style)
- `php artisan test --compact` — Pest suite

## OAuth

ATProto OAuth is the only auth mechanism — no passwords, email, or registration. The user row stores `did` (primary key), encrypted `refresh_token`, and `iss` (auth server URL). Handle is resolved at login and lives in the Laravel session, never the DB. Profile data (avatar, display name) is re-fetched live from the PDS when needed.

Scopes: granular `repo:app.skyreader.*` per-collection, following [Dan Abramov's guidance](https://underreacted.leaflet.pub/3mjfozhlhys2z). Avoid `transition:generic`.

Client metadata is served at `/oauth-client-metadata.json`, JWKS at `/oauth-jwks.json`, callback at `/oauth/callback` (route name `bluesky.oauth.redirect`, package convention).

## Key References

- [SPEC.md](./SPEC.md) — product spec
- [ATProto OAuth](https://atproto.com/specs/oauth), [ATProto permissions](https://atproto.com/specs/permission)
- [revolution/laravel-bluesky docs](https://github.com/invokable/laravel-bluesky/blob/main/docs/socialite.md)
- Skyreader lexicons — `lexicons/app/skyreader/`
