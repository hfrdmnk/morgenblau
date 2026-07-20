import { describe, expect, test } from 'bun:test';

import { countLabel } from './plural';

describe('countLabel', () => {
    test('uses the singular noun for exactly one', () => {
        expect(countLabel(1, 'source', 'sources')).toBe('1 source');
    });

    test('uses the plural noun for zero', () => {
        expect(countLabel(0, 'source', 'sources')).toBe('0 sources');
    });

    test('uses the plural noun for many', () => {
        expect(countLabel(4, 'person', 'people')).toBe('4 people');
    });
});
