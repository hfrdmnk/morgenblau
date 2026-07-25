import { describe, expect, test } from 'bun:test';

import type { Profile } from './profile';
import { hydrateNetworkShares, uniqueSharerDIDs, type NetworkShare } from './library';

function share(overrides: Partial<NetworkShare> = {}): NetworkShare {
    return {
        sharerDid: 'did:plc:alice',
        kind: 'rss',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

function profile(overrides: Partial<Profile> = {}): Profile {
    return {
        did: 'did:plc:alice',
        handle: 'reader.example',
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

describe('hydrateNetworkShares', () => {
    test('attaches the resolved profile to each share by sharer DID', () => {
        const shares = [
            share({ sharerDid: 'did:plc:alice' }),
            share({ sharerDid: 'did:plc:bob' }),
        ];
        const dids = ['did:plc:alice', 'did:plc:bob'];
        const profiles = [
            profile({ did: 'did:plc:alice', handle: 'alice.example' }),
            profile({ did: 'did:plc:bob', handle: 'bob.example' }),
        ];

        const hydrated = hydrateNetworkShares(shares, dids, profiles);

        expect(hydrated[0]?.profile?.handle).toBe('alice.example');
        expect(hydrated[1]?.profile?.handle).toBe('bob.example');
    });

    test('leaves profile undefined when a lookup failed', () => {
        const shares = [share({ sharerDid: 'did:plc:alice' })];
        const dids = ['did:plc:alice'];
        const profiles = [undefined];

        const hydrated = hydrateNetworkShares(shares, dids, profiles);

        expect(hydrated[0]?.profile).toBeUndefined();
    });

    test('repeated sharers all resolve to the same profile', () => {
        const shares = [
            share({ sharerDid: 'did:plc:alice' }),
            share({ sharerDid: 'did:plc:alice', createdAt: '2026-07-02T00:00:00Z' }),
        ];
        const dids = ['did:plc:alice'];
        const profiles = [profile({ handle: 'alice.example' })];

        const hydrated = hydrateNetworkShares(shares, dids, profiles);

        expect(hydrated[0]?.profile?.handle).toBe('alice.example');
        expect(hydrated[1]?.profile?.handle).toBe('alice.example');
    });
});
