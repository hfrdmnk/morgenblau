import type { Transition } from 'motion/react';

// Single source of truth for the split/merge duration: the value tweens (marginTop, radii, height) and the phase timers both read it.
export const SPLIT_DURATION_MS = 320;

// Duration of the divider full-bleed pre-phase and its retract on merge.
export const DIVIDER_MS = 140;

const OPACITY_FADE_MS = 220;

// Cut pop reveal (reason avatars, head actions): pops trail the split so each element lands in an already-open slot.
const POP_MS = 160;
const POP_DELAY_MS = 60;
const POP_STAGGER_MS = 50;

// Conceal cascade is compressed so each element departs ~25ms before the folding width reaches its slot.
const CONCEAL_MS = 110;
const CONCEAL_STAGGER_MS = 30;

// easeInOutCubic: an on-screen morph is two-sided by design; house rule is ripple, no spring/overshoot.
const SPLIT_EASE: [number, number, number, number] = [0.645, 0.045, 0.355, 1];
const FADE_EASE: [number, number, number, number] = [0.23, 1, 0.32, 1];

export const split = (): Transition => ({
    duration: SPLIT_DURATION_MS / 1000,
    ease: SPLIT_EASE,
});

const opacityFade = (): Transition => ({
    duration: OPACITY_FADE_MS / 1000,
    ease: FADE_EASE,
});

// Opening geometry releases with ease-out so motion reads the instant the cut commits;
// closing keeps the two-sided settle (split), decelerating into the merge.
export const splitOpen = (): Transition => ({
    duration: SPLIT_DURATION_MS / 1000,
    ease: FADE_EASE,
});

export const splitFade = (): Transition => ({ ...split(), opacity: opacityFade() });

export const splitOpenFade = (): Transition => ({ ...splitOpen(), opacity: opacityFade() });

export const cutStaggerPop = (order: number): Transition => ({
    duration: POP_MS / 1000,
    delay: (POP_DELAY_MS + order * POP_STAGGER_MS) / 1000,
    ease: FADE_EASE,
});

// Reverse of the entrance stagger: the last element in is the first out (outOrder 0).
export const cutConcealPop = (outOrder: number): Transition => ({
    duration: CONCEAL_MS / 1000,
    delay: (outOrder * CONCEAL_STAGGER_MS) / 1000,
    ease: FADE_EASE,
});

// Instant retarget on the cut and merge commits so persisting cards snap before tweening.
export const CUT_SNAP: Transition = { duration: 0 };
