import { describe, expect, test } from 'bun:test';

import {
    randomHueShift,
    randomSeedWord,
} from '../src/lib/random-values';

describe('random values', () => {
    test('maps random values across the full hue-shift range', () => {
        expect(randomHueShift(() => 0)).toBe(-180);
        expect(randomHueShift(() => 0.999999)).toBe(180);
    });

    test('returns a single dictionary word for a seed', () => {
        expect(randomSeedWord(() => 0)).toMatch(/^[a-z]+$/);
        expect(randomSeedWord(() => 0.999999)).toMatch(/^[a-z]+$/);
    });
});
