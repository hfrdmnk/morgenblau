# Morgenblau

A calm content platform powered by RSS and ATProto — daily digests instead of infinite feeds.

## Spec Compliance

[SPEC.md](./SPEC.md) is the source of truth for product vision, content model, and guardrails. All code must follow the spec. When you notice a discrepancy between the code and the spec, flag it and either ask to update the spec or adapt the code — never silently diverge.

## Tooling

- **Package manager: bun only.** Never use npm, yarn, or pnpm. All install/run/build commands must use `bun`.
- **Never read or search inside `node_modules/`.** Treat it as off-limits.
- **Never use inline eval** (`node -e`, `bun -e`, `node run -e`, `bun run -e`, or any equivalent). Do not create temporary files to work around this restriction either. To investigate package APIs or types, use [npmx.dev](https://npmx.dev) instead.

## Testing

- Use **Red/Green TDD** as the standard development paradigm with **Vitest** as the test runner.

## Verification

After each batch of work, run all three checks:

- `bun run lint` — ESLint (0 errors, 0 warnings)
- `bun run types` — TypeScript type checking
- `bun run test` — Vitest test suite

## UI Primitives

### Window

`src/components/Window.tsx` is the app's core layout primitive — a fixed, inset rectangle with asymmetric radii (larger at the top, smaller at the bottom) that acts as the viewport into every screen. Both logged-out and logged-in screens render their content inside a `<Window>`. When the user says "the window", this is it. Content scrolls inside the window; the window itself is fixed.

## Key References

- [SPEC.md](./SPEC.md) — product spec
- [ATProto docs](https://atproto.com/) — protocol documentation for AT Protocol integration
