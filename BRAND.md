# Morgenblau

Morgenblau is a personal daily web newspaper. It gathers sources someone chooses into finite days, creating time to read without an infinite feed or pressure to catch up.

## Character

The product is calm, clear, and deliberate. It should feel connected to the world without demanding attention. Hierarchy comes primarily from whitespace and tonal contrast, keeping content comfortable to scan and read.

Newspaper describes the product model, not a visual costume. Avoid paper textures, ornamental rules, old printing effects, and nostalgic newspaper styling. Do not call a day or digest an edition.

## Visual direction

The interface is a quiet, tonal field built from Stone neutrals. It uses little elevation, few borders, and no decorative chrome. Ordinary actions remain neutral. Atmosphere blue is the one vivid brand color; it marks the active mode, whichever mode that is. Its lighter companion exists only as a gradient endpoint. A few semantic colors carry meaning rather than brand, such as the yellow star on primary sources and the violet wand on generated feeds; they never mark a mode.

Geist is the interface typeface and default reader font. Newsreader and iA Writer Duo (labelled Mono) are optional reader-body faces; titles and metadata remain Geist. The major-second scale and the visual tokens live in [`app/frontend/src/index.css`](app/frontend/src/index.css); components choose size and weight independently.

## Art direction

Brand imagery can be richer than the interface. It draws from nature, light, and atmosphere, then abstracts those references through broad color fields, soft focus, restrained grain, and implied motion. The result should preserve the mood of a real place without relying on literal scenery or polished landscape photography.

[`hfrdmnk/abstract-studio`](https://github.com/hfrdmnk/abstract-studio) owns the image-generation treatment. This document owns its role in the brand, not its implementation.

## Language

Use **newspaper** for the finite daily collection.

Use **curate** and **curation** for choosing sources. Do not call people editors. Never use **edition** for product concepts.

The voice is calm, direct, and human. No urgency, no exclamation marks, no emoji, and no language about being behind. Never use unread counts or progress to pressure people.

## The mark

The mark is an abstract sun rising over the ocean. It connects Morgenblau to light, the horizon, and the feeling of a fresh start. It does not mean that every brand image needs to show a sunrise.

Wherever the mark sits beside text, the text stands on the horizon: the bottom edge of the horizon bar lines up with the text's baseline, and the notch below it hangs like a descender. Icons on the same line centre on the text, not on the mark. The site footer in [`site/src/pages/index.astro`](site/src/pages/index.astro) shows the technique.
