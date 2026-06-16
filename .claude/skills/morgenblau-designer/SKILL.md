---
name: morgenblau-designer
description: Morgenblau's UI design language for all interface work: surface layers, control tokens, color tokens, typography (Geist sans for product, Newsreader serif for the reader), radius, motion, icons, and anti-patterns. Brand truths (essence, metaphors, voice, color story) live in BRAND.md; this skill implements them. Use whenever designing or building Morgenblau UI: new components, new screens, layout changes, or any edit to how Morgenblau looks or moves. Invoke before choosing classNames, sizes, colors, or radii, and when reviewing a Morgenblau PR for visual consistency.
---

# Morgenblau designer

This skill is Morgenblau's **UI design language**: how the interface looks and moves. The brand layer (essence, the Edition and Morning ideas, voice and tone, the color story, the mark and wordmark) lives in `BRAND.md` at the repo root. This skill implements it.

Implementation details (file paths, CSS tokens, React primitives) live in `src/` and in `plans/morgenblau-design-implementation.md`. For _how_ to realize a rule, look there.

Keep the craft bar high (taste, not concrete examples): Linear's precision, Family / Benji Taylor's warmth, Emil Kowalski's restraint, Josh Puckett's animation care, Dieter Rams' "less but better." If a UI decision doesn't serve the brand in `BRAND.md`, simplify it away.

---

## Surface layers — closer = _lighter_

Morgenblau inverts the common "shadows signal elevation" pattern. Closeness to the user is expressed by **luminance**, not shadow. Surfaces rise toward the user by getting lighter.

| Level                  | Light surface | Light border | Dark surface | Dark border | Example                                                        |
| ---------------------- | ------------- | ------------ | ------------ | ----------- | -------------------------------------------------------------- |
| **0** — base           | gray-100      | —            | gray-950     | —           | the page background, behind everything                         |
| **1** — one above base | gray-50       | gray-200     | gray-900     | gray-800    | a card sitting on bare base (e.g. the login card). |
| **2** — two above base | white         | gray-100     | gray-800     | gray-700    | a card on the page (e.g. a digest card).                 |

**Principle:** the same direction in both modes — closer is lighter. In light mode the ladder climbs toward white. In dark mode it climbs from near-black toward mid-gray.

**Level is a property of the element, not its container.** A surface primitive takes its level explicitly. Default is `2` (the common case — a card on the page). A card placed directly on the base gets level `1`. This makes surfaces predictable and keeps the same primitive working in framed and bare-base contexts.

**Do not** stack deeper than two levels above base. If you find yourself needing a level 3, you've composed too many cards — restructure.

---

## Controls invert the layer system

Surfaces lighten as they come forward. **Controls do the opposite: they step forward by contrasting _against_ the surface brightness trend.** Two axes, opposite directions.

| Role                    | Rule                               | L0 light                                                                                         | L1 light | L2 light | L0 dark  | L1 dark  | L2 dark  |
| ----------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------ | -------- | -------- | -------- | -------- | -------- |
| **Input bg**            | one step past surface              | gray-50                                                                                          | gray-100 | gray-50  | gray-700 | gray-800 | gray-700 |
| **Input border**        | two steps past                     | gray-200                                                                                         | gray-200 | gray-100 | gray-600 | gray-700 | gray-600 |
| **Secondary button bg** | two steps past                     | gray-100                                                                                         | gray-200 | gray-100 | gray-600 | gray-700 | gray-600 |
| **Primary button**      | color-defined, not level-dependent | solid atmosphere-blue background, white foreground, no border — same in both modes               |          |          |          |          |          |

**In light mode controls go darker; in dark mode they go lighter.** The rule holds: controls always stand _against_ the surface trend, so they visually step forward.

**Exception — level 0 (base) inverts the inversion.** On the darkest surface, there is no darker step available in light mode (and no lighter step in dark mode), so the rule flips: on L0, controls go **lighter** than the surface, not darker. On L0 the input **bg** matches the L2 bg, but the **border goes a step darker than L2's** (gray-200 in light mode) so the edge still reads against the base; dark mode reuses the L2 border (gray-600) unchanged because it already stands out against gray-950.

**Focus state — different rule for buttons and inputs.** Morgenblau draws its own focus indicator on the element the user actually perceives — not always the natively focusable one. The shape of the indicator depends on the control:

- **Buttons and clickable controls — 1 px outline, offset 2 px, atmosphere-blue.** Pattern: `outline-none` base, then `focus-visible:outline-solid focus-visible:outline-1 focus-visible:outline-offset-2 focus-visible:outline-ring`. `outline-solid` is explicit — Tailwind v4 has no bare `outline` utility. The result looks native: thin, solid, sits *outside* the control. Use this on `<Button>`, links, icon buttons, anything you click.
- **Inputs and compound input wrappers — border color swap, no outline, no ring.** Pattern: `border-input` base, then `focus-visible:border-ring` on a bare input, or `has-[[data-slot=input-group-control]:focus-visible]:border-ring` on a compound wrapper. The border itself becomes atmosphere-blue. No outer halo, no box-shadow, no offset outline — the field's edge already exists, so the focus indicator just colors it. Stays inside the field's footprint and reads as a single calm shape.

Why split: an outline on a control with a visible border looks like two parallel rings (border + outline), which feels noisy. Buttons (typically borderless or border-transparent) need a separate shape; inputs already have one.

Error / invalid states (`aria-invalid`) swap the border to `border-destructive` on inputs, and the existing destructive ring style on buttons — so focus and validation stay distinguishable.

**Two button variants only: primary (solid atmosphere-blue, white text) and secondary (gray).** There is no dark / black variant. When a button needs to feel more critical, the answer is **copy and placement**, not a louder color. A confirmation word, a more prominent position, a soft hint underneath — but never a third button variant.

---

## Color

Morgenblau's palette is almost entirely monochrome, with two exceptions: one **brand accent** and four **category markers**.

| Token             | Role                                                                                    |
| ----------------- | --------------------------------------------------------------------------------------- |
| `atmosphere-blue` | the one non-monochrome accent — primary actions, focus rings, highlights. Nothing else. |
| `leaf-green`      | longform / blogpost category                                                            |
| `sunset-orange`   | micropost category                                                                      |
| `coral-red`       | video category                                                                          |
| `aurora-purple`   | podcast category                                                                        |
| `sand-brown`      | **sunrise-horizon only** — gradient endpoints. Never text. Never surfaces.              |

**Rules of use:**

- `atmosphere-blue` is for intent: the primary action, a focus ring, a moment of discovery. Not for decoration, not for chrome, not for backgrounds.
- Category colors appear **only on their content type**. A leaf-green swatch on anything that isn't longform breaks the system.
- `sand-brown` appears **only** as the bottom endpoint of a sunrise gradient (e.g. the login panel). Not as a surface tone, not as a text color, not as a border.
- Grays do all the structural work: surfaces, borders, controls, typography.

---

## Typography

**Geist** carries the entire product UI — chrome, digest, forms, settings, navigation. **Newsreader** (variable serif, `opsz` + `wght` axes) is the project's reading font, used inside the long-form article reader for body copy only. Titles and captions inside the reader stay Geist; the size delta between sans title and serif body is the visual cue that "you're now reading."

**Two font colors, ever:**

- **Primary** (darker) — headings, body text
- **Secondary** (lighter, `muted-foreground`) — hints, metadata, fine print

Scale is **calm, not dramatic**. h1 is not 40 px. Hierarchy is built more with weight than size.

**Four weights, ever.** No bold (700+). Hierarchy is carried by the delta between weights plus size plus tracking, not by heavy strokes.

| Weight          | Role                                                                                         |
| --------------- | -------------------------------------------------------------------------------------------- |
| 300 — light     | secondary / muted text — hints, captions, timestamps, fine print                             |
| 400 — regular   | body, paragraphs, labels, metadata, UI chrome text — the default voice                       |
| 500 — medium    | small / mid-size headings (h3–h6), card titles, long-form titles                             |
| 600 — semibold  | page-level headings (h1 / h2) on auth and entry surfaces — used sparingly to anchor a screen |

**These defaults are wired into base styles in `app.css` and apply automatically by tag.** Plain `<h1>`, `<h2>`, … pick up size + weight + tracking without any className. Plain `<p>` picks up `font-normal`. Override only when the semantic tag doesn't match the role (e.g. a `<span>` acting as a heading) or when stepping a paragraph down to light for muted/secondary copy. **Don't repeat the defaults in className** — if you find yourself writing `<h1 className="text-2xl font-semibold tracking-tight">`, delete those classes.

| Token                  | Size      | Weight | Tracking         | Use                                                       |
| ---------------------- | --------- | ------ | ---------------- | --------------------------------------------------------- |
| `text-xs`              | 0.75 rem  | 300    | normal           | tiny meta (timestamps, fine print)                        |
| `text-sm`              | 0.875 rem | 300    | normal           | secondary text, hints                                     |
| `text-sm`              | 0.875 rem | 400    | normal           | labels, inline UI text                                    |
| `text-base`            | 1 rem     | 400    | normal           | body                                                      |
| `text-lg`              | 1.125 rem | 500    | `tracking-tight` | small headings, card titles                               |
| `text-xl`              | 1.25 rem  | 500    | `tracking-tight` | section headings (h3)                                     |
| `text-2xl`             | 1.5 rem   | 600    | `tracking-tight` | page headings (h1 / h2) — `font-semibold`                 |
| `font-serif text-base` | 1 rem     | 400    | normal           | **reader body only.** Newsreader for long-form paragraphs |
| `font-serif text-sm`   | 0.875 rem | 300    | normal           | **reader captions only.** Figure captions, fine print     |

**Rules:**

- h1 caps at 1.5 rem (about 1.5× body). Whispered, not shouted — but at this size, semibold (600) is what carries the page; medium would feel too soft against the calm scale.
- **Headings always use `tracking-tight` (-0.025em)** — body stays at normal tracking. Tightening once, gently, is a craft signature; tightening more (`tracking-tighter`) reads as anxious, not calm. Don't escalate.
- In long-form text (article reader), the **font shift itself** carries hierarchy: title in Geist (`font-sans`, medium 500), body in Newsreader (`font-serif`, regular 400). Sans-to-serif is the cue. Inside a single voice (the body) the same weight-not-size rule still applies — drop to light (300) only for genuinely *secondary* copy (caption under a figure, blockquote attribution).
- **Newsreader is reader-only.** Never used in product UI chrome (digest, forms, settings, navigation, dialogs); never used for titles, captions, or metadata outside the long-form reader. If you find yourself reaching for `font-serif` in a non-reader surface, you've broken the metaphor — the serif appears only after the user has chosen to read.
- Newsreader's optical-size axis (`opsz` 9pt–60pt) auto-targets the rendered size; the variable font picks an appropriate optical cut without per-tag overrides. Don't set `font-variation-settings` manually unless you have a specific reason.

---

## Radius — ladder scaled to element size

| Size                       | Use                                 |
| -------------------------- | ----------------------------------- |
| `rounded-sm` (0.125 rem)   | tags, chips                         |
| `rounded-lg` (0.5 rem)     | small badges, icon wells            |
| `rounded-xl` (0.75 rem)    | **buttons, inputs**                 |
| `rounded-2xl` (1 rem)      | tight containers                    |
| `rounded-3xl` (1.5 rem)    | secondary cards                     |
| `rounded-4xl` (2 rem)      | **primary cards** (e.g. login card) |

**Principle: radius scales with element size.** A 2 rem radius on a button makes it a lozenge; a 0.75 rem radius on a hero card makes it uptight. Match the roundness to the surface area.

The overall UI should feel generously rounded — never square, never pill-shaped by default. Everything stays on the symmetric ladder.

---

## The reader view — Newsreader's only home

The long-form article reader is the one place a user comes to **read**, not to scan. It is also the only place Newsreader appears.

**Why Newsreader, why only here.** Sans-serif is right for product UI: scanning a digest, picking a source, navigating settings. Serif is right for sustained reading: paragraphs of prose at body size for minutes at a time. Newsreader is purpose-built for that — its variable optical-size axis tunes letterforms to the rendered point size, and its proportions read calmly at body sizes on screen. Letting the typeface shift between scanning and reading turns the reader view into a felt place, not just a different layout.

**Inside the reader:**

- **Title:** Geist (`font-sans`), medium (500), `tracking-tight`. Stays sans so the article header reads as continuous with the rest of the product chrome — the reader is a place inside Morgenblau, not a separate brand.
- **Byline / meta line:** Geist (`font-sans`), light (300) or regular (400), `text-muted-foreground`. Source name, author, relative date, time-to-read.
- **Body paragraphs:** Newsreader (`font-serif`), regular (400), default tracking. This is the only place serif appears in the entire product.
- **Blockquotes, asides, captions:** Newsreader (`font-serif`), light (300) for figure captions; serif italic (the variable italic axis) is acceptable inside a blockquote. Stay in the serif voice — switching back to sans inside the body fragments the reading surface.
- **Inline emphasis:** italic (variable italic) for `<em>`, semibold (600) for `<strong>` — both in Newsreader. Don't bring back sans inside a paragraph.

**Forbidden uses of `font-serif`:**

- Anywhere outside the article reader page
- Page headings, section headings, button labels, navigation, form labels
- Empty-state copy (lives in Geist + light + muted)
- Marketing chrome, error messages, settings copy, dialog titles

If you're tempted to use `font-serif` to "make this feel more bookish" outside the reader, you've found a marketing / decoration urge — resist. The serif's weight as a metaphor depends on it being scarce.

---

## Motion

**Defer to the `web-animation-design` skill for the fundamentals** — `ease-out` for enter/exit, `ease-in-out` for on-screen morphing, `ease` for hovers, `transform` + `opacity` only, `prefers-reduced-motion` on every animation, under 300 ms for UI, no animation for 100×/day actions. That skill holds the base rules; don't re-derive them here.

**Morgenblau-specific rules that layer on top:**

1. **Ripple, not spring.** Default: no bounce, no overshoot. Stricter than the baseline — springs are actively avoided on product surfaces.
2. **Springs are reserved for exactly two situations:**
   - **Social affirmations** — like, save, follow. Subtle Apple-style `{ duration: 0.4, bounce: 0.2 }` maximum. The one place delight lands in motion.
   - **New edition arrival** — when the morning, lunch, or evening fetch lands, the first card of the new edition settles in with a whisper of spring. **Once per edition**, not per card.
3. **Two duration tokens:**
   - **UI duration (~180 ms)** — hovers, button press, dropdowns, tooltips, input focus, card hover lift.
   - **Scene duration (~320 ms)** — article open, edition switch, day change, auth → home transition.
4. **Article open.** When an article opens, a new surface sheets up from below with `ease-out-quint`, ~320 ms, `translateY(100%) → 0`. Dismissal is faster (~260 ms) with `ease-out-cubic`. Exits are always faster than entrances.
5. **What never animates:**
   - Calendar strip horizontal scroll when driven by arrow keys (used daily; must feel instant)
   - Day number re-render inside a cell
   - Focus ring appearance (must be visible immediately)
6. **Anti-patterns:**
   - **Staggered card entrances on digest load.** Digest rows appear together — no waterfall reveal. Reinforces "a finite object, all arrived at once."
   - **Parallax or scroll-driven effects on cards.** Content is the point; decoration isn't.
   - **Pulsing skeleton screens.** Use a soft fade-in when content resolves, not a pulse.

---

## Voice and tone

Lives in `BRAND.md` at the repo root. Not duplicated here.

---

## Icons

- **Library:** Hugeicons (React).
- **Variant: stroke.** Solid and two-tone feel heavy next to the calm typography.
- **Sizes:** 1 rem (tight inline, e.g. inside small buttons or chip adornments), 1.125 rem (standard inline with body text), 1.25 rem (primary navigation, app chrome).
- **Stroke weight:** Hugeicons' stroke default (1.5 px). Do not override.
- **Color: `currentColor`.** Icons inherit the text color of their surrounding context — primary, secondary, or atmosphere-blue depending on where they sit. Icons almost never need their own color token.
- **Pairing with text:** `inline-flex items-center gap-[0.5em]`.

---

## Anti-patterns (the list)

If a Morgenblau design exhibits any of these, something has gone wrong:

- **Pure-white body background.** The base is `gray-100`; white belongs to level-2 cards.
- **Dramatic type scale** (h1 at 2+ rem, big jumps between levels). Everything above 1.5 rem reads as shouting.
- **Atmosphere-blue used decoratively** (backgrounds, chrome, non-functional surfaces). Blue is for intent; grays do structure.
- **Category colors outside their content type.** A leaf-green swatch on a video card breaks the system.
- **`sand-brown` as a text or surface color.** It's the sunrise-horizon endpoint — nothing else.
- **Spring physics by default on UI motion.** The motion metaphor is ripples settling, not springs bouncing.
- **Staggered card entrances on digest load.** All content arrives together.
- **A third button variant** (solid, black, dark). Two variants carry every action. Critical actions earn emphasis through copy and placement, not louder buttons.
- **Newsreader (or any serif) in product UI.** Serif belongs to long-form reader body copy only. Using `font-serif` for titles, captions, UI chrome, or marketing breaks the metaphor; the reader stops feeling like a place. (The brand wordmark is a separate lockup set in Newsreader italic; see `BRAND.md`.)
- **Hand-drawn, script, or decorative _typefaces_.** The product type is Geist + Newsreader; no third typeface. (The editor's-hand marks defined in `BRAND.md` are illustration, not type, and are allowed where that doc permits them.)
- **Unread counts, progress indicators, "X items left" badges.** These are anti-Morgenblau.