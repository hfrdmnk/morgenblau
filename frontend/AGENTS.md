---
paths:
  - "frontend/**"
---

# Frontend conventions

## Stack

- Bun only: `bun run lint`, `bun run build`, `bunx` for one-offs. Never npm/npx/pnpm.
- UI components in `src/components/ui/` are shadcn **base-nova** (Base UI primitives via `@base-ui/react` subpath imports), not the Radix variant. House additions: `data-slot`, `cn()`, the surface-level system.
- Icons are ProIcons (`@proicons/react`, direct named components), never lucide, never hugeicons.
- Design language: invoke the `morgenblau-designer` skill before any visual work; brand truths live in BRAND.md.

## Adding shadcn components

- `bunx shadcn@latest add <name>` from `frontend/`. Never run `shadcn init` (it rewrites `index.css` tokens).
- The CLI writes into a literal `frontend/@/` directory; move wanted files to `src/components/ui/` and delete `./@`.
- It may overwrite house components listed as registry deps. Review the diff and restore house versions; never pass `--yes` blindly.
- Swap any lucide imports the generated code carries for the ProIcons equivalents.

## Lint rules that fail the build (react-hooks v7)

- Never assign `ref.current` during render; sync in an effect instead.
- Never `setState` inside `useEffect` to react to a prop change; use the adjust-during-render pattern (compare against a `prev` state in the render body).
- A file exporting a component cannot also export hooks/functions; shared hooks go in `src/hooks/`.
- Never add `eslint-disable` comments; fix the underlying issue.

## API contract

- Backend errors are JSON `{code, message}` with optional `errors` (field map). Reauth is exactly `403` plus `code: "reauth_required"`; missing-or-not-owned resources are `404` on every verb.
- All API calls go through `api()` from `src/lib/api.ts`; never call `fetch` inline. It throws `ApiError` (`status`, `code`, `message`, `errors`, `.isReauth`); pass `signal` for cancellable calls. Best-effort profile lookups live in `src/lib/profile.ts`.

## Interaction patterns

- List row highlight is one CSS element that travels (~150ms) across both hover and keyboard selection, driven by a unified active index. Never a JS animation library for this.
- `motion` (motion.dev) tweens real geometry values for the discover card break-up (margins/radii/height via the tokens in `src/lib/motion-transitions.ts`), not layout projection. Always pass an explicit transition (its default layout spring violates the no-spring rule), wrap motion trees in `MotionConfig reducedMotion="user"`, and drive animated values through `animate`, never a spring.

## Codebase hygiene

- fallow gates every commit via `.githooks/pre-commit` (`fallow audit`, new-only: only findings the changeset introduces block). Config lives in `frontend/.fallowrc.jsonc`; fresh clones activate hooks with `make hooks`.
- shadcn `ui/` components are exempt from unused-export rules there; don't delete their unwired subcomponents.
- Run `bun run doctor` (react-doctor) regularly: at least once per feature branch before it merges. Treat its diagnostics as hypotheses to verify in the code, not verdicts.
- Whole-tree sweeps beyond the commit gate: `bunx fallow` (dead code, duplication, complexity, boundaries).

## Testing

- Generic fixtures only. Test data uses reserved namespaces and invented names, never real people, handles, domains, or publications.

## Verification

- `cd frontend && bun run lint && bun run build` before considering frontend work done.
