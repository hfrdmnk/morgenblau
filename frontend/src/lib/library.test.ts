import { describe, expect, test } from 'bun:test';

import { uniqueSharerDIDs, type NetworkShare } from './library';

function share(overrides: Partial<NetworkShare> = {}): NetworkShare {
    return {
        sharerDid: 'did:plc:alice',
        kind: 'rss',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

describe('uniqueSharerDIDs', () => {
    test('dedupes repeated sharers while preserving first-seen order', () => {
        const shares = [
            share({ sharerDid: 'did:plc:alice' }),
            share({ sharerDid: 'did:plc:bob' }),
            share({ sharerDid: 'did:plc:alice' }),
        ];
        expect(uniqueSharerDIDs(shares)).toEqual(['did:plc:alice', 'did:plc:bob']);
    });

    test('empty input yields an empty list', () => {
        expect(uniqueSharerDIDs([])).toEqual([]);
    });
});
