import { describe, expect, test } from 'bun:test';

import { metaLine, readAuthor } from './entry-meta';

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

describe('metaLine', () => {
    test('joins present parts with a middot', () => {
        expect(metaLine(['Jun 11, 2026', 'example.com'])).toBe(
            'Jun 11, 2026 · example.com',
        );
    });

    test('drops null, undefined, and empty parts', () => {
        expect(metaLine([null, 'example.com', undefined, ''])).toBe(
            'example.com',
        );
    });

    test('returns null when nothing is present', () => {
        expect(metaLine([])).toBeNull();
        expect(metaLine([null, undefined])).toBeNull();
    });
});
