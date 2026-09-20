import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';

import type { Save } from './library';
import {
    readSavedCache,
    writeCachedSaves,
    writeSavedCache,
} from './library-cache';
import { emitLibraryMutation } from './library-events';

const HOUR_MS = 60 * 60 * 1000;

function save(overrides: Partial<Save> = {}): Save {
    return {
        rkey: '3lasavealpha',
        itemUrl: 'https://news.example.com/posts/one',
        createdAt: '2026-07-01T00:00:00Z',
        title: 'Example Article',
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
});

describe('the 1h TTL', () => {
    test('expires the saves entry', () => {
        writeSavedCache([save()]);
        expect(readSavedCache()?.saves).toEqual([save()]);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readSavedCache()).toBeUndefined();
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
});

describe('library mutation events', () => {
    test('a save clears the saves entry', () => {
        writeSavedCache([save()]);

        emitLibraryMutation();

        expect(readSavedCache()).toBeUndefined();
    });
});
