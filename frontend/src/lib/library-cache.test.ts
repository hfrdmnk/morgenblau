import { afterEach, describe, expect, setSystemTime, test } from 'bun:test';

import type { Save } from './library';
import {
    readSavedCache,
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

describe('the 1h TTL', () => {
    test('expires the saves entry', () => {
        writeSavedCache([save()]);
        expect(readSavedCache()?.saves).toEqual([save()]);
        setSystemTime(Date.now() + HOUR_MS + 1);
        expect(readSavedCache()).toBeUndefined();
    });
});

describe('library mutation events', () => {
    test('a save clears the saves entry', () => {
        writeSavedCache([save()]);

        emitLibraryMutation();

        expect(readSavedCache()).toBeUndefined();
    });
});
