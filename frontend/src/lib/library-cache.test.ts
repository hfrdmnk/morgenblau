import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';

import type { NetworkShare, Save, Share } from './library';
import {
    readNetworkCache,
    readSavedCache,
    readSharedCache,
    writeCachedNetworkProfiles,
    writeCachedNetworkShares,
    writeCachedSaves,
    writeCachedShares,
    writeNetworkCache,
    writeSavedCache,
    writeSharedCache,
} from './library-cache';
import { emitLibraryMutation } from './library-events';

const HOUR_MS = 60 * 60 * 1000;
const ALICE = 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa';
const BOB = 'did:plc:bbbbbbbbbbbbbbbbbbbbbbbb';

function save(overrides: Partial<Save> = {}): Save {
    return {
        rkey: '3lasavealpha',
        itemUrl: 'https://news.example.com/posts/one',
        createdAt: '2026-07-01T00:00:00Z',
        title: 'Example Article',
        ...overrides,
    };
}

function share(overrides: Partial<Share> = {}): Share {
    return {
        rkey: '3lasharealpha',
        kind: 'rss',
        itemUrl: 'https://news.example.com/posts/two',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

function networkShare(overrides: Partial<NetworkShare> = {}): NetworkShare {
    return {
        sharerDid: ALICE,
        kind: 'rss',
        itemUrl: 'https://news.example.com/posts/three',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

afterEach(() => {
    setSystemTime();
});

// Runs before any seeding write in this file establishes an entry.
describe('with no cache entries yet', () => {
    test('writeCachedSaves is a no-op', () => {
        writeCachedSaves([save()]);
        expect(readSavedCache()).toBeUndefined();
    });

    test('writeCachedShares is a no-op', () => {
        writeCachedShares([share()]);
        expect(readSharedCache()).toBeUndefined();
    });

    test('writeCachedNetworkShares is a no-op', () => {
        writeCachedNetworkShares([networkShare()]);
        expect(readNetworkCache()).toBeUndefined();
    });

    test('writeCachedNetworkProfiles is a no-op', () => {
        writeCachedNetworkProfiles({
            [ALICE]: { did: ALICE, handle: 'alice.example' },
        });
        expect(readNetworkCache()).toBeUndefined();
    });
});

describe('the 1h TTL', () => {
    test('expires the saves entry', () => {
        writeSavedCache([save()]);
        expect(readSavedCache()?.saves).toEqual([save()]);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readSavedCache()).toBeUndefined();
    });

    test('expires the shares entry', () => {
        writeSharedCache([share()]);
        expect(readSharedCache()?.shares).toEqual([share()]);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readSharedCache()).toBeUndefined();
    });

    test('expires the network entry', () => {
        writeNetworkCache([networkShare()]);
        expect(readNetworkCache()?.shares).toEqual([networkShare()]);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readNetworkCache()).toBeUndefined();
    });
});

describe('partial writers', () => {
    test('writeCachedSaves replaces the list without refreshing the TTL', () => {
        writeSavedCache([save()]);
        const fetchedAt = readSavedCache()?.fetchedAt;
        setSystemTime(Date.now() + 1_000);

        const updated = save({ title: 'Updated Article' });
        writeCachedSaves([updated]);

        expect(readSavedCache()?.saves).toEqual([updated]);
        expect(readSavedCache()?.fetchedAt).toBe(fetchedAt);
    });

    test('writeCachedShares replaces the list without refreshing the TTL', () => {
        writeSharedCache([share()]);
        const fetchedAt = readSharedCache()?.fetchedAt;
        setSystemTime(Date.now() + 1_000);

        writeCachedShares([]);

        expect(readSharedCache()?.shares).toEqual([]);
        expect(readSharedCache()?.fetchedAt).toBe(fetchedAt);
    });

    test('writeCachedNetworkProfiles merges into the seeded entry', () => {
        const alice = { did: ALICE, handle: 'alice.example' };
        const bob = { did: BOB, handle: 'bob.example' };
        writeNetworkCache([networkShare()]);

        writeCachedNetworkProfiles({ [ALICE]: alice });
        writeCachedNetworkProfiles({ [BOB]: bob });

        expect(readNetworkCache()?.profiles).toEqual({
            [ALICE]: alice,
            [BOB]: bob,
        });
        expect(readNetworkCache()?.shares).toEqual([networkShare()]);
    });

    test('writeNetworkCache resets previously resolved profiles', () => {
        writeNetworkCache([networkShare()]);
        writeCachedNetworkProfiles({
            [ALICE]: { did: ALICE, handle: 'alice.example' },
        });

        writeNetworkCache([networkShare()]);

        expect(readNetworkCache()?.profiles).toEqual({});
    });
});

describe('library mutation events', () => {
    test('a save clears the saves entry and leaves the others', () => {
        writeSavedCache([save()]);
        writeSharedCache([share()]);
        writeNetworkCache([networkShare()]);

        emitLibraryMutation({ kind: 'save' });

        expect(readSavedCache()).toBeUndefined();
        expect(readSharedCache()?.shares).toEqual([share()]);
        expect(readNetworkCache()?.shares).toEqual([networkShare()]);
    });

    test('a share clears the shares and network entries and leaves saves', () => {
        writeSavedCache([save()]);
        writeSharedCache([share()]);
        writeNetworkCache([networkShare()]);

        emitLibraryMutation({ kind: 'share' });

        expect(readSharedCache()).toBeUndefined();
        expect(readNetworkCache()).toBeUndefined();
        expect(readSavedCache()?.saves).toEqual([save()]);
    });
});
