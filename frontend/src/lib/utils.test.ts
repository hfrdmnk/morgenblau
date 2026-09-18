import { describe, expect, test } from 'bun:test';

import { isPlainLeftClick } from './utils';

const click = (overrides: Partial<Parameters<typeof isPlainLeftClick>[0]>) => ({
    button: 0,
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    ...overrides,
});

describe('isPlainLeftClick', () => {
    test('accepts an unmodified left click', () => {
        expect(isPlainLeftClick(click({}))).toBe(true);
    });

    test('rejects modifier keys', () => {
        expect(isPlainLeftClick(click({ metaKey: true }))).toBe(false);
        expect(isPlainLeftClick(click({ ctrlKey: true }))).toBe(false);
        expect(isPlainLeftClick(click({ shiftKey: true }))).toBe(false);
    });

    test('rejects middle and right clicks', () => {
        expect(isPlainLeftClick(click({ button: 1 }))).toBe(false);
        expect(isPlainLeftClick(click({ button: 2 }))).toBe(false);
    });
});
