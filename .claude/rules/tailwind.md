---
paths: ['**/*.{css,scss,vue,tsx,jsx}']
---

# Tailwind CSS v4 Rules

### Core Changes from v3

- **CSS-first config** — no `tailwind.config.js` by default
- **Single import** — `@import "tailwindcss"` replaces `@tailwind base/components/utilities`
- **Auto content detection** — no `content` array needed
- **Built-in Lightning CSS** — no `autoprefixer` or `postcss-import` needed
- **5x faster builds**, 100x+ faster incremental

### Setup

**Vite**: `import tailwindcss from "@tailwindcss/vite"; export default { plugins: [tailwindcss()] }`

**PostCSS**: `export default { plugins: { "@tailwindcss/postcss": {} } }`

**CSS**: `@import "tailwindcss"`

### \@theme Directive

All customization in CSS via `@theme`. Variables auto-generate utilities:

```css
@theme {
	--color-brand: #3b82f6; /* → bg-brand, text-brand */
	--font-display: 'Inter'; /* → font-display */
	--spacing-18: 4.5rem; /* → p-18, m-18, gap-18 */
	--breakpoint-3xl: 1920px; /* → 3xl:flex */
	--radius-pill: 9999px; /* → rounded-pill */
	--animate-fade-in: fade-in 0.3s ease-out;
	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
}
```

**Override defaults**: `--color-*: initial;`. **Reference vars**: `@theme inline` (no :root variable)

### Directives Quick Reference

| Directive                         | Purpose                      |
| --------------------------------- | ---------------------------- |
| `@import "tailwindcss"`           | Load Tailwind                |
| `@theme { }`                      | Define theme variables       |
| `@config "./file.js"`             | Load JS config (migration)   |
| `@source "../path"` / `not`       | Add/exclude content paths    |
| `@plugin "@tailwindcss/forms"`    | Load plugins                 |
| `@utility name { }`               | Custom utility with variants |
| `@variant dark { }`               | Apply variant in CSS         |
| `@custom-variant name (selector)` | Define custom variant        |
| `@reference "../app.css"`         | Reference in scoped styles   |

### Custom Utilities

**MUST use `@utility` for variant support** (not `@layer utilities`):

```css
@utility content-auto {
	content-visibility: auto;
} /* Works with hover:, dark:, lg: */

@utility tab-* {
	/* Functional utilities */
	tab-size: --value(--tab-size-*, integer); /* → tab-2, tab-4, tab-[8] */
}
```

### Layers

Order: `theme → base → components → utilities`

```css
@layer components {
	.card {
		background: var(--color-white);
		border-radius: var(--radius-lg);
		padding: var(--spacing-6);
	}
}
```

### Renamed Utilities (v3 → v4)

| v3                      | v4                                          |
| ----------------------- | ------------------------------------------- |
| `outline-none`          | `outline-hidden`                            |
| `ring`                  | `ring-3` (default now 1px)                  |
| `bg-gradient-to-r`      | `bg-linear-to-r`                            |
| `bg-opacity-50`         | `bg-black/50`                               |
| `border` (gray default) | `border border-gray-200` (now currentColor) |

### New Utilities

- **Container queries**: `@container`, `@sm:`, `@lg:`, `@max-md:`
- **3D transforms**: `rotate-x-*`, `rotate-y-*`, `perspective-*`, `transform-3d`
- **Shadows**: `inset-shadow-*`, `inset-ring-*`
- **Field sizing**: `field-sizing-content` (auto-resize textarea)
- **Gradients**: `bg-linear-45`, `bg-conic`, `bg-radial-[at_25%_25%]`
- **Color scheme**: `scheme-light`, `scheme-dark`

### New Variants

| Variant    | Use                           |
| ---------- | ----------------------------- |
| `not-*`    | `not-hover:opacity-75`        |
| `in-*`     | Like group-\* without class   |
| `nth-*`    | `nth-3:bg-red-500`            |
| `starting` | `starting:open:opacity-0`     |
| `inert`    | `inert:opacity-50`            |
| `**`       | Descendant: `**:text-red-500` |

**Stacked variants**: Order changed to left-to-right: `*:first:pt-0` (v4) vs `first:*:pt-0` (v3)

### CSS Variable Syntax Change

v3: `bg-[--brand-color]` → v4: `bg-(--brand-color)`

### Dark Mode

**System preference**: Works by default, just use `dark:`

**Class-based**: `@custom-variant dark (&:where(.dark, .dark *))`

**Data attribute**: `@custom-variant dark (&:where([data-theme="dark"], [data-theme="dark"] *))`

### Migration Checklist

Run `npx @tailwindcss/upgrade`, then verify:

1. `@tailwind` → `@import "tailwindcss"`, renamed utils (shadow/rounded/blur/ring)
2. `bg-opacity-*` → `bg-color/opacity`, CSS vars `[--var]` → `(--var)`
3. Reverse stacked variants, explicit `border-gray-200`, `ring` → `ring-3`
4. Remove autoprefixer/postcss-import, `tailwindcss` → `@tailwindcss/postcss`
5. Convert `tailwind.config.js` to `@theme`

### Styling Preference Order

1. **Theme utilities** over arbitrary: `text-secondary` vs `text-[var(--color-text-secondary)]`
2. **Arbitrary values** over style attr: `text-[#123456]` vs `style="color: #123456"`

### Common Mistakes

| Mistake                              | Fix                     |
| ------------------------------------ | ----------------------- |
| `@layer utilities` for custom        | `@utility name { }`     |
| `tailwindcss` as PostCSS plugin      | `@tailwindcss/postcss`  |
| Creating `tailwind.config.js`        | Use `@theme` in CSS     |
| `bg-opacity-50`                      | `bg-black/50`           |
| `bg-[--var]`                         | `bg-(--var)`            |
| Adding autoprefixer                  | Built-in, remove it     |
| `@tailwind base`                     | `@import "tailwindcss"` |
| `text-[var(--color-text-secondary)]` | `text-secondary`        |
