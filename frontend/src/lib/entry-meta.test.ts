import { describe, expect, test } from 'bun:test';

import { readAuthor } from './entry-meta';

describe('readAuthor', () => {
    test('returns the author string from metadata JSON', () => {
        expect(readAuthor('{"author":"Example Author"}')).toBe(
            'Example Author',
        );
    });

    test('returns null for missing metadata', () => {
        expect(readAuthor(null)).toBeNull();
        expect(readAuthor(undefined)).toBeNull();
        expect(readAuthor('')).toBeNull();
    });

    test('returns null when author is absent or not a string', () => {
        expect(readAuthor('{}')).toBeNull();
        expect(readAuthor('{"author":42}')).toBeNull();
    });

    test('returns null for invalid JSON', () => {
        expect(readAuthor('not json')).toBeNull();
    });
});
