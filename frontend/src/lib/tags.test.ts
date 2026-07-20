import { describe, expect, test } from 'bun:test';

import { mergeTagSuggestions } from './tags';

describe('mergeTagSuggestions', () => {
    test('unions multiple lists, sorted', () => {
        expect(mergeTagSuggestions(['banana'], ['apple'])).toEqual([
            'apple',
            'banana',
        ]);
    });

    test('dedupes case-insensitively, keeping the first-seen casing', () => {
        expect(mergeTagSuggestions(['Tech'], ['tech', 'TECH'])).toEqual([
            'Tech',
        ]);
    });

    test('handles a single list', () => {
        expect(mergeTagSuggestions(['b', 'a'])).toEqual(['a', 'b']);
    });

    test('handles no lists or all-empty lists', () => {
        expect(mergeTagSuggestions()).toEqual([]);
        expect(mergeTagSuggestions([], [])).toEqual([]);
    });
});
