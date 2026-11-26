---
paths:
    - 'app/frontend/**'
---

# Frontend conventions

## Stack

- Bun only: `bun run <script>`, `bunx` for one-offs, never npm/npx/pnpm. CI rejects any other lockfile. Vite runs on Bun's runtime (`bun --bun`) and the build type-checks with `bun check`; ESLint stays on Node because Bun gives it no speedup.
- `src/components/ui/` builds on Base UI primitives from `@base-ui/react`, styled with the same tokens as the rest of the app.
- Central Icons is the only icon source: round, outlined, 2px radius, 1.5px stroke; the selected mobile tab and the primary and generated source marks use the filled set. `src/components/icons.tsx` maps them to app names, and only it imports `@central-icons-react/*`, which `eslint.config.js` enforces. Never copy Central SVG markup into the repo: the license forbids redistributing it.
- Brand direction lives in the root `BRAND.md`; visual tokens live in `src/index.css`.

## Design system

- Markup outside `src/components/ui/` and `src/design/` is `Frame`, `Text`, `MotionFrame` and components; their props take tokens from `src/design/tokens.ts`, never class strings. The rules in `eslint/design-system.js` enforce it, with their cases in `eslint/design-system.test.ts`. `/__dev/styleguide` renders every token.
- When no token or variant fits, add one: a token to `tokens.ts` (then `bun run design:tokens`), a value with both themes to `src/index.css`, or a variant to the component in `src/components/ui/`. Never reach for an exception.
- Class strings in `src/components/ui/` follow the same token rules: no arbitrary values or properties, opacity modifiers, `dark:` or palette colours. A one-off value becomes a CSS variable in `src/index.css` read with `prop-(--name)`, and a multi-declaration recipe becomes an `@utility` there.
- Every exception is a commented `files` glob in `eslint.config.js`; inline disables already fail. A growing list means the design system is missing something.
- `Frame` is a flex column with no margins: siblings are spaced by the parent's `gap`. Text inside a `Frame` goes in a `Text`, or it becomes its own flex item.

## Motion

- Motion is for feedback and temporary context, in keeping with `BRAND.md`'s calm character: nothing decorative, looping or attention-seeking. Loading skeletons are the exception: their slow pulse, in `.skeleton` in `src/index.css`. The `/__dev/styleguide` Motion section shows the rules in use.
- CSS transitions take their durations from the `--motion-duration-*` tokens and easing from `--ease-*` in `src/index.css`, and list the properties they transition instead of `transition-all`.
- Animate `transform`, `opacity` and `filter`. Size and position changes go through `motion`'s `layout`, or the keyframes in `src/index.css` and `tw-animate-css`; never transition width, height or offsets by hand. `eslint.config.js` rejects `transition-all` and size or offset transitions in class strings outside `src/components/ui/`.
- Reduced motion is part of every animation: the tokens collapse under `prefers-reduced-motion`, CSS animations sit behind `motion-safe:` or `motion-reduce:`, and `motion/react` code reads `useReducedMotion()` and goes instant.
- Multi-stage choreography is a storyboard: a header comment listing each stage with its time, then a `TIMING` object and per-element config objects that hold every duration, easing and spring, never inline in JSX. `src/components/add-source-dialog.tsx` is the reference. The pattern is adapted from the Storyboard Animation guide in Josh Puckett's Interface Craft.
- `test/preload.ts` explains how `motion` runs under happy-dom.

## Adding primitives

- Write new primitives by hand in `src/components/ui/` on a `@base-ui/react` part, with variants in `class-variance-authority` and icons from `src/components/icons.tsx`. Add a specimen to `/__dev/styleguide`.

## PWA

- `vite.config.ts` and `src/pwa.test.ts` own static-asset precaching, the offline shell and the offline image cache; the service worker stores no other `/api` response. Reads opt into device copies with `api(path, { offline: true })`, under SPEC.md's device-copy rule. Offline mutation sync remains outside scope.

## Lint

- `eslint.config.js` and the `lint` script in `package.json` own enforcement; fix violations in code.
- Shared hooks live in `src/hooks/`, since a component file may export only components.

## Loading

- Skeletons come from `Skeleton` in `src/components/ui/skeleton.tsx`: wrap what renders once data arrives, with a `sample` next to the component. Never hand-draw placeholder bars; `eslint.config.js` rejects `animate-pulse` outside `src/components/ui/`.
- `.skeleton` in `src/index.css` paints the sample as bars with CSS alone. Keep it free of JavaScript layout reads: a skeleton mounts on every navigation, before the first paint.
- Content rendered from a `sample` mounts inert and hidden from screen readers while loading, so effects with visible side effects, like `useDocumentTitle`, belong outside the `Skeleton`.
- A page shows its own skeleton, never a generic "Loading" screen. Import pages eagerly. A `Suspense` fallback is `null` or the page's `*Skeleton`; `eslint.config.js` enforces that and rejects `lazy()` page imports.

## API

- `api()` in `src/lib/api.ts` is the API boundary; `eslint.config.js` owns its enforcement. Review cancellable calls for a passed `signal`.

## Codebase hygiene

- fallow gates every commit through `.githooks/pre-commit` (only findings the changeset introduces block); config lives in `.fallowrc.jsonc`, and fresh clones activate hooks with `make hooks`.
- Run `bun run doctor` (react-doctor) at least once per feature branch before it merges. Treat its diagnostics as hypotheses to verify in the code, not verdicts.
- Whole-tree sweeps beyond the commit gate: `bunx fallow` (dead code, duplication, complexity, boundaries).

## Testing

- Fixtures follow the Testing rule in the root `AGENTS.md`. The `verify-morgenblau` skill says which checks a change needs.
