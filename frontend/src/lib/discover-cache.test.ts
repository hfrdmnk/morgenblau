import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';

import type { DiscoverSource, DiscoverSourcePost } from '@/lib/discover';
import type { Profile } from '@/lib/profile';

import {
    readDiscoverCache,
    writeCachedPost,
    writeCachedProfiles,
    writeCachedSources,
    writeDiscoverCache,
} from './discover-cache';

const HOUR_MS = 60 * 60 * 1000;

function source(overrides: Partial<DiscoverSource> = {}): DiscoverSource {
    return {
        key: 'https://feed.example.com/rss.xml',
        kind: 'rss',
        title: 'Example Source',
        reason: { strongCount: 0, weakCount: 0 },
        ...overrides,
    };
}

function post(overrides: Partial<DiscoverSourcePost> = {}): DiscoverSourcePost {
    return {
        key: 'https://feed.example.com/rss.xml#1',
        title: 'Example Post',
        publishedAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

const profiles: Record<string, Profile | undefined> = {};

afterEach(() => {
    setSystemTime();
});

// Runs before any writeDiscoverCache call in this file establishes an entry.
describe('with no cache entry yet', () => {
    test('writeCachedSources is a no-op', () => {
        writeCachedSources([source()], undefined);
        expect(readDiscoverCache()).toBeUndefined();
    });

    test('writeCachedPost is a no-op', () => {
        writeCachedPost(source().key, [post()]);
        expect(readDiscoverCache()).toBeUndefined();
    });

    test('writeCachedProfiles is a no-op', () => {
        writeCachedProfiles({});
        expect(readDiscoverCache()).toBeUndefined();
    });
});

describe('readDiscoverCache', () => {
    test('a fresh entry is readable', () => {
        writeDiscoverCache([source()], profiles, 'cursor-1');
        expect(readDiscoverCache()?.sources).toEqual([source()]);
        expect(readDiscoverCache()?.nextCursor).toBe('cursor-1');
    });

    test('an entry older than the 1h TTL reads as undefined', () => {
        writeDiscoverCache([source()], profiles);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readDiscoverCache()).toBeUndefined();
    });
});

describe('writeCachedSources', () => {
    test('write-through updates sources and cursor without touching fetchedAt or posts', () => {
        writeDiscoverCache([source()], profiles, 'cursor-1');
        const fetchedAt = readDiscoverCache()?.fetchedAt;
        writeCachedPost(source().key, [post()]);

        const updated = source({ title: 'Updated Source' });
        writeCachedSources([updated], 'cursor-2');

        const entry = readDiscoverCache();
        expect(entry?.sources).toEqual([updated]);
        expect(entry?.nextCursor).toBe('cursor-2');
        expect(entry?.posts[source().key]).toEqual([post()]);
        expect(entry?.fetchedAt).toBe(fetchedAt);
    });
});

describe('writeCachedProfiles', () => {
    test('merges newly resolved profiles without touching posts', () => {
        const alice = { did: 'did:plc:alice', handle: 'alice.test' };
        const bob = { did: 'did:plc:bob', handle: 'bob.test' };
        writeDiscoverCache([source()], { [alice.did]: alice });
        writeCachedPost(source().key, [post()]);

        writeCachedProfiles({ [bob.did]: bob });

        expect(readDiscoverCache()?.profiles).toEqual({
            [alice.did]: alice,
            [bob.did]: bob,
        });
        expect(readDiscoverCache()?.posts[source().key]).toEqual([post()]);
    });
});

describe('writeCachedPost', () => {
    test('stores posts under the given key', () => {
        writeDiscoverCache([source()], profiles);
        writeCachedPost(source().key, [post()]);
        expect(readDiscoverCache()?.posts[source().key]).toEqual([post()]);
    });
});

describe('writeDiscoverCache', () => {
    test('resets previously written posts', () => {
        writeDiscoverCache([source()], profiles);
        writeCachedPost(source().key, [post()]);
        expect(readDiscoverCache()?.posts[source().key]).toEqual([post()]);

        writeDiscoverCache([source()], profiles);
        expect(readDiscoverCache()?.posts).toEqual({});
    });
});
