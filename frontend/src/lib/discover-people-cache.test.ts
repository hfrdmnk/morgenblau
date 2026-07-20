import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';

import type { DiscoverPerson, PersonPreview } from '@/lib/discover-people';
import type { FollowRecord } from '@/lib/follow';
import type { Profile } from '@/lib/profile';

import {
    readPeopleCache,
    writeCachedFollows,
    writeCachedPeople,
    writeCachedPeopleProfiles,
    writeCachedPersonPreview,
    writePeopleCache,
} from './discover-people-cache';

const HOUR_MS = 60 * 60 * 1000;

function person(overrides: Partial<DiscoverPerson> = {}): DiscoverPerson {
    return {
        did: 'did:plc:alice',
        reason: {
            blueskyFollow: true,
            tangledFollow: false,
            sharedSourceCount: 0,
        },
        ...overrides,
    };
}

function follow(overrides: Partial<FollowRecord> = {}): FollowRecord {
    return {
        rkey: '3kabc',
        subjectDid: 'did:plc:bob',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

function preview(overrides: Partial<PersonPreview> = {}): PersonPreview {
    return { writes: [], reads: [], latestShare: null, ...overrides };
}

const profiles: Record<string, Profile | undefined> = {};

afterEach(() => {
    setSystemTime();
});

// Runs before any writePeopleCache call in this file establishes an entry.
describe('with no cache entry yet', () => {
    test('writeCachedPeople is a no-op', () => {
        writeCachedPeople([person()], undefined);
        expect(readPeopleCache()).toBeUndefined();
    });

    test('writeCachedFollows is a no-op', () => {
        writeCachedFollows([follow()]);
        expect(readPeopleCache()).toBeUndefined();
    });

    test('writeCachedPersonPreview is a no-op', () => {
        writeCachedPersonPreview(person().did, preview());
        expect(readPeopleCache()).toBeUndefined();
    });

    test('writeCachedPeopleProfiles is a no-op', () => {
        writeCachedPeopleProfiles({});
        expect(readPeopleCache()).toBeUndefined();
    });
});

describe('readPeopleCache', () => {
    test('a fresh entry is readable', () => {
        writePeopleCache([person()], [follow()], profiles, 'cursor-1');
        const entry = readPeopleCache();
        expect(entry?.people).toEqual([person()]);
        expect(entry?.follows).toEqual([follow()]);
        expect(entry?.nextCursor).toBe('cursor-1');
    });

    test('an entry older than the 1h TTL reads as undefined', () => {
        writePeopleCache([person()], [follow()], profiles);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readPeopleCache()).toBeUndefined();
    });
});

describe('writeCachedPeople', () => {
    test('write-through updates people and cursor without touching fetchedAt or previews', () => {
        writePeopleCache([person()], [follow()], profiles, 'cursor-1');
        const fetchedAt = readPeopleCache()?.fetchedAt;
        writeCachedPersonPreview(person().did, preview());

        const updated = person({ did: 'did:plc:carol' });
        writeCachedPeople([updated], 'cursor-2');

        const entry = readPeopleCache();
        expect(entry?.people).toEqual([updated]);
        expect(entry?.nextCursor).toBe('cursor-2');
        expect(entry?.previews[person().did]).toEqual(preview());
        expect(entry?.fetchedAt).toBe(fetchedAt);
    });
});

describe('writeCachedPeopleProfiles', () => {
    test('merges newly resolved profiles without touching previews', () => {
        const alice = { did: 'did:plc:alice', handle: 'alice.test' };
        const bob = { did: 'did:plc:bob', handle: 'bob.test' };
        writePeopleCache([person()], [follow()], { [alice.did]: alice });
        writeCachedPersonPreview(alice.did, preview());

        writeCachedPeopleProfiles({ [bob.did]: bob });

        expect(readPeopleCache()?.profiles).toEqual({
            [alice.did]: alice,
            [bob.did]: bob,
        });
        expect(readPeopleCache()?.previews[alice.did]).toEqual(preview());
    });
});

describe('writeCachedFollows', () => {
    test('write-through updates follows without touching fetchedAt', () => {
        writePeopleCache([person()], [follow()], profiles);
        const fetchedAt = readPeopleCache()?.fetchedAt;

        const updated = follow({ rkey: '3kxyz' });
        writeCachedFollows([updated]);

        const entry = readPeopleCache();
        expect(entry?.follows).toEqual([updated]);
        expect(entry?.fetchedAt).toBe(fetchedAt);
    });
});

describe('writeCachedPersonPreview', () => {
    test('stores a preview under the given DID', () => {
        writePeopleCache([person()], [follow()], profiles);
        const value = preview({
            reads: [
                {
                    key: 'https://feed.example.com/rss.xml',
                    kind: 'rss',
                    title: 'Example Weekly',
                    siteUrl: 'https://example.com',
                    subscribed: false,
                },
            ],
        });
        writeCachedPersonPreview('did:plc:alice', value);
        expect(readPeopleCache()?.previews['did:plc:alice']).toEqual(value);
    });
});

describe('writePeopleCache', () => {
    test('resets previously written previews', () => {
        writePeopleCache([person()], [follow()], profiles);
        writeCachedPersonPreview('did:plc:alice', preview());
        expect(readPeopleCache()?.previews['did:plc:alice']).toEqual(preview());

        writePeopleCache([person()], [follow()], profiles);
        expect(readPeopleCache()?.previews).toEqual({});
    });
});
