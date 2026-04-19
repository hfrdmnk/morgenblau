---
name: morgenblau-designer
description: Morgenblau's complete design language for all UI work — surface layers, control tokens, color rules, typography, radius, the Window primitive, Caveat annotations, motion, voice, icons, anti-patterns. Use this skill whenever designing or building Morgenblau UI — new components, new screens, layout changes, or any edit that touches how Morgenblau looks, feels, sounds, or moves. Invoke before choosing classNames, sizes, colors, radii, or writing UI copy for Morgenblau. Also invoke when reviewing a Morgenblau PR for visual or tonal consistency.
---

# Morgenblau designer

This skill encodes the design language for Morgenblau — a calm content platform powered by RSS and ATProto, delivering daily digests instead of infinite feeds. The goal of every design decision is the product's core promise: **intentionality without deprivation.**

This is **design language only**. Implementation details (file paths, CSS tokens, React primitives) live in `src/` and in `plans/morgenblau-design-implementation.md`. If you need to know _how_ to realize a rule, look there — not here.

---

## North star

- **80 / 20.** 80 % of the surface area should feel clean, minimal, straight-to-the-point. The remaining 20 % is personality — delivered through copy, Caveat annotations, and subtle motion. Never through louder color or extra decoration.
- **Texture: crisp morning.** The terrace on a clear morning, not the candlelit cafe. Clean sans-serifs, cool blues, precise transitions, generous whitespace.
- **Craft reference cluster** (taste, not concrete examples): Linear's precision, Family / Benji Taylor's warmth, Emil Kowalski's restraint, Josh Puckett's animation care, Dieter Rams' "less but better."
- **Motion metaphor: ripples settling on water, not springs.** Ease-out dominant. Overshoot and bounce are reserved for rare delight moments.
- **The window, and the newspaper.** Two metaphors. The Window is something you choose to look through, then step away from. The newspaper is the feeling of a finite object with a start and an end — a ritual, not a habit. Never an infinite feed, never an inbox, never an "unread count."
- **Identity.** The user isn't managing subscriptions. They're the **editor of their own daily publication.**

If a design decision doesn't clearly serve one of these, simplify it away.

---

## Surface layers — closer = _lighter_

Morgenblau inverts the common "shadows signal elevation" pattern. Closeness to the user is expressed by **luminance**, not shadow. Surfaces rise toward the user by getting lighter.

| Level                  | Light surface | Light border | Dark surface | Dark border | Example                                                        |
| ---------------------- | ------------- | ------------ | ------------ | ----------- | -------------------------------------------------------------- |
| **0** — base           | gray-100      | —            | gray-950     | —           | the page background, behind everything                         |
| **1** — one above base | gray-50       | gray-200     | gray-900     | gray-800    | the Window. A card sitting on bare base (e.g. the login card). |
| **2** — two above base | white         | gray-100     | gray-800     | gray-700    | a card inside the Window (e.g. a digest card).                 |

**Principle:** the same direction in both modes — closer is lighter. In light mode the ladder climbs toward white. In dark mode it climbs from near-black toward mid-gray.

**Level is a property of the element, not its container.** A surface primitive takes its level explicitly. Default is `2` (the common case — a card inside the Window). A card placed directly on the base gets level `1`. This makes surfaces predictable and keeps the same primitive working in Window-framed and bare-base contexts.

**Do not** stack deeper than two levels above base. If you find yourself needing a level 3, you've composed too many cards — restructure.

---

## Controls invert the layer system

Surfaces lighten as they come forward. **Controls do the opposite: they step forward by contrasting _against_ the surface brightness trend.** Two axes, opposite directions.

| Role                    | Rule                               | L1 light                                                                                         | L2 light | L1 dark  | L2 dark  |
| ----------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------ | -------- | -------- | -------- |
| **Input bg**            | one step past surface              | gray-100                                                                                         | gray-50  | gray-800 | gray-700 |
| **Input border**        | two steps past                     | gray-200                                                                                         | gray-100 | gray-700 | gray-600 |
| **Secondary button bg** | two steps past                     | gray-200                                                                                         | gray-100 | gray-700 | gray-600 |
| **Primary button**      | color-defined, not level-dependent | soft atmosphere-blue tint in both modes (filled tint, tinted border, atmosphere-blue foreground) |          |          |          |

**In light mode controls go darker; in dark mode they go lighter.** The rule holds: controls always stand _against_ the surface trend, so they visually step forward.

**Two button variants only: primary (soft blue) and secondary (gray).** There is no solid / dark / black variant. When a button needs to feel more critical, the answer is **copy and placement**, not a louder color. A confirmation word, a more prominent position, a Caveat hint — but never a third button variant.

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

**Harmony to preserve:** `atmosphere-blue`, `leaf-green`, and `sunset-orange` share lightness and chroma (only the hue rotates). This is intentional — they sit side by side in a digest without one dominating. `coral-red` and `aurora-purple` are slightly punchier so video and podcast, which are more attention-seeking formats, earn a bit more visual pull. Do not break this harmony when introducing new colors; extend the same chroma + lightness pattern if a future category arrives.

**Rules of use:**

- `atmosphere-blue` is for intent: the primary action, a focus ring, a moment of discovery. Not for decoration, not for chrome, not for backgrounds.
- Category colors appear **only on their content type**. A leaf-green swatch on anything that isn't longform breaks the system.
- `sand-brown` appears **only** as the bottom endpoint of a sunrise gradient (e.g. the login panel). Not as a surface tone, not as a text color, not as a border.
- Grays do all the structural work: surfaces, borders, controls, typography.

---

## Typography

**Geist** for almost everything. **Caveat** for annotations — and only annotations (see the annotation section).

**Two font colors, ever:**

- **Primary** (darker) — headings, body text
- **Secondary** (lighter, `muted-foreground`) — hints, metadata, all Caveat

Scale is **calm, not dramatic**. h1 is not 40 px. Hierarchy is built more with weight than size.

| Token       | Size      | Weight | Tracking           | Use                         |
| ----------- | --------- | ------ | ------------------ | --------------------------- |
| `text-xs`   | 0.75 rem  | 400    | normal             | tiny meta (timestamps)      |
| `text-sm`   | 0.875 rem | 400    | normal             | secondary text, hints       |
| `text-base` | 1 rem     | 400    | normal             | body                        |
| `text-lg`   | 1.125 rem | 500    | `tracking-tight`   | small headings, card titles |
| `text-xl`   | 1.25 rem  | 600    | `tracking-tight`   | section headings (h3)       |
| `text-2xl`  | 1.5 rem   | 700    | `tracking-tighter` | page headings (h1 / h2)     |

**Rules:**

- h1 caps at 1.5 rem (about 1.5× body). Whispered, not shouted.
- **Tracking tightens as size grows** — body is normal, mid-size headings are `tracking-tight` (-0.025em), large headings are `tracking-tighter` (-0.05em). Tight tracking at scale is a craft signature; it reads as intentional.
- In long-form text (article reader), titles and paragraphs differ mainly by **weight**, not size. Bolder paragraphs, not dramatically larger ones.
- Caveat renders visually smaller than Geist at the same nominal size — bump it up one step when placed beside Geist body.
- **Caveat is always secondary color.** Never primary. Never atmosphere-blue. Never a category color.

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
| asymmetric 4 rem / 0.5 rem | **Window only**                     |

**Principle: radius scales with element size.** A 2 rem radius on a button makes it a lozenge; a 0.75 rem radius on a hero card makes it uptight. Match the roundness to the surface area.

The overall UI should feel generously rounded — never square, never pill-shaped by default. The Window's asymmetric radii are the one exception; everything else stays on the symmetric ladder.

---

## The Window

Morgenblau's core layout metaphor. Two invariants.

**1. Asymmetric radii.** Top ~4 rem (opens skyward); bottom ~0.5 rem (grounded). Symmetric radii read as a modal. The asymmetry is the point — the Window sits on something, and opens toward something.

**2. One per screen.** Either:

- framing primary content (e.g. the digest), or
- serving as a decorative anchor (e.g. the sunrise sky panel on the login screen).

Never both on the same screen. Never two Windows stacked or side by side. When an article opens, it **replaces** (sheets over) the current Window rather than layering on top — the metaphor stays singular.

The Window's insets, positioning, and scroll behavior are **consumer-level** concerns and vary by context. They do not belong in this skill; treat them as an implementation detail of whatever layout is hosting the Window.

---

## Caveat annotations — the reader's pencil

Caveat is a small, warm hand laid on top of a crisp page. It's the reader's pencil mark, the editor's note in the margin. To keep it from becoming decoration, Caveat is allowed in exactly four places and forbidden everywhere else.

**Allowed uses (always secondary color):**

1. **Input hints** — one line below a form field. Short. Example: _"New here? Enter your handle without `@`."_
2. **Content meta** — time-to-read, relative date, source name next to a card. Example: _"4 min · 2 days ago"_ next to a Geist title.
3. **Margin asides** on long-form reading surfaces only (article reader, settings pages, docs) — a short pencil-mark thought in the margin. Never on digest rows, cards, or forms.
4. **Empty-state personality** — the second line of an empty-state block can be Caveat. Example: _"Nothing new this morning."_ (Geist body) / _"Enjoy your coffee."_ (Caveat, smaller, secondary).

**Forbidden uses:**

- As a button label
- As a heading or section title
- Inside buttons, cards, or any interactive surface label
- For navigation, tabs, or menu items
- As the primary copy in any block — Caveat always accompanies Geist body that carries the information; it never carries the information alone

**Never:** hand-drawn decorations (circles, arrows, underlines) outside long-form reading surfaces. They belong to marketing and long-form contexts, not to the app chrome or digest.

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
   - **Scene duration (~320 ms)** — Window → sheet (article open), edition switch, day change, auth → home transition.
4. **Window sheet-replacement.** When an article opens, a new Window sheets up from below with `ease-out-quint`, ~320 ms, `translateY(100%) → 0`. Dismissal is faster (~260 ms) with `ease-out-cubic`. Exits are always faster than entrances.
5. **What never animates:**
   - Calendar strip horizontal scroll when driven by arrow keys (used daily; must feel instant)
   - Day number re-render inside a cell
   - Focus ring appearance (must be visible immediately)
6. **Anti-patterns:**
   - **Staggered card entrances on digest load.** Digest rows appear together — no waterfall reveal. Reinforces "a finite object, all arrived at once."
   - **Parallax or scroll-driven effects on cards.** Content is the point; decoration isn't.
   - **Pulsing skeleton screens.** Use a soft fade-in when content resolves, not a pulse.

---

## Voice & tone

**Register:** calm and direct, like a thoughtful newspaper editor. No corporate voice. No cheerleader voice. No LLM voice ("I'll help you..."). One step warmer than Linear, one step cooler than Superhuman.

**Rules:**

1. **Short sentences.** Most copy fits in one line. Two lines only if the second is a gentle afterthought (the Caveat line).
2. **Second person is default.** _"You curated this"_ — not _"Users can curate..."_
3. **No exclamation marks. Ever.** The product is calm; the copy mirrors it.
4. **No emoji.** Ever.
5. **No "unread," no counts, no urgency language.** _"Just 3 left to read!"_ is anti-Morgenblau.
6. **Empty states are features.** Name the calm, don't apologize for it.
7. **Errors are matter-of-fact.** _"Couldn't reach that feed. We'll try again at lunch."_ No "Oops," "Uh oh," or "Something went wrong."
8. **Prefer concrete over abstract.** _"Morning edition"_ beats _"New content digest."_
9. **First-party language.** _"Your publication"_ — not _"your account."_ _"Sources"_ — not _"subscriptions."_ The user is the editor, not a subscriber.

**The voice test:** _Would a calm editor with an espresso say this at 7 am without performing?_ If no, rewrite.

---

## Icons

- **Library:** Hugeicons (React).
- **Variant: stroke.** Solid and two-tone feel heavy next to the calm typography.
- **Sizes:** 1 rem (tight inline, e.g. inside small buttons or chip adornments), 1.125 rem (standard inline with body text), 1.25 rem (primary navigation, Window chrome).
- **Stroke weight:** Hugeicons' stroke default (1.5 px). Do not override.
- **Color: `currentColor`.** Icons inherit the text color of their surrounding context — primary, secondary, or atmosphere-blue depending on where they sit. Icons almost never need their own color token.
- **Pairing with text:** `inline-flex items-center gap-[0.5em]`.

---

## Anti-patterns (the list)

If a Morgenblau design exhibits any of these, something has gone wrong:

- **Pure-white body background.** The base is `gray-100`; white belongs to level-2 cards.
- **Symmetric Window corners.** The top/bottom asymmetry is the metaphor. Equal radii = modal, not Window.
- **More than one Window per screen.** The metaphor is singular.
- **Dramatic type scale** (h1 at 2+ rem, big jumps between levels). Everything above 1.5 rem reads as shouting.
- **Atmosphere-blue used decoratively** (backgrounds, chrome, non-functional surfaces). Blue is for intent; grays do structure.
- **Category colors outside their content type.** A leaf-green swatch on a video card breaks the system.
- **`sand-brown` as a text or surface color.** It's the sunrise-horizon endpoint — nothing else.
- **Spring physics by default on UI motion.** The motion metaphor is ripples settling, not springs bouncing.
- **Staggered card entrances on digest load.** All content arrives together.
- **Hand-drawn decoration on app chrome, digest rows, or forms.** Those belong to marketing and long-form surfaces only.
- **A third button variant** (solid, black, dark). Two variants carry every action. Critical actions earn emphasis through copy and placement, not louder buttons.
- **Caveat as primary copy, button label, heading, or nav item.** Caveat accompanies Geist; it never carries meaning alone.
- **Unread counts, progress indicators, "X items left" badges.** These are anti-Morgenblau.
- **Exclamation marks or emoji in UI copy.** Breaks the voice.
