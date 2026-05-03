---
paths:
    - "app/**/*.php"
    - "resources/js/**/*.{ts,tsx}"
---

Use `spatie/laravel-data` for any DTO, value object, or response payload. Place Data classes under `app/Data/{Topic}/` with the `…Data` suffix, mark fields `public readonly`, add `#[TypeScript]` so the transformer picks them up, and type enum-like fields with the matching backed enum from `app/Enums/` instead of string literals.

Wire-format mappers depend on direction. For payloads emitted to the frontend, add `#[MapOutputName(SnakeCaseMapper::class)]` so JSON keys land as snake_case. For payloads built from incoming snake_case input (form requests, JSON bodies), add `#[MapInputName(SnakeCaseMapper::class)]` so `Data::from($validated)` populates camelCase properties without manual remapping.

In TypeScript, never hand-roll interfaces or types that mirror a backend shape. Consume types from `resources/js/types/generated.d.ts` (emitted by `spatie/laravel-typescript-transformer` via `php artisan typescript:transform`; the generated file is gitignored, run the command after pulling). Reference them as `App.Data.{Topic}.{Name}Data` or `App.Enums.{Name}`. If the type you need isn't generated yet, add `#[TypeScript]` to the PHP `Data` class and re-run the transformer — don't author the TS by hand.
