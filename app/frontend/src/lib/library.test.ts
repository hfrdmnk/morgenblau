import { describe, expect, test } from 'bun:test';

import { libraryTab, queueGroups, saveState, visibleTags, withState, type LibrarySave } from '@/lib/library';

describe('Library states as tags', () => {
    test('a save is to read until it carries the archived tag, in any case', () => {
        expect(saveState([])).toBe('queue');
        expect(saveState(['design', 'kept'])).toBe('queue');
        expect(saveState(['design', 'ARCHIVED'])).toBe('archived');
    });

    test('the state tag never shows as the reader’s own tag', () => {
        expect(visibleTags(['Archived', 'design', 'kept', 'archived-ideas'])).toEqual([
            'design',
            'kept',
            'archived-ideas',
        ]);
    });

    test('filing replaces the state and keeps the reader’s tags in order', () => {
        expect(withState(['design', 'Archived', 'cooking'], 'archived')).toEqual(['archived', 'design', 'cooking']);
        expect(withState(['archived', 'design'], 'queue')).toEqual(['design']);
        expect(withState([], 'archived')).toEqual(['archived']);
    });

    test('the tab comes from the URL and falls back to To read', () => {
        expect(libraryTab('?tab=archived')).toBe('archived');
        expect(libraryTab('?tab=kept')).toBe('queue');
        expect(libraryTab('')).toBe('queue');
    });
});

describe('queue groups', () => {
    const save = (title: string, extra: { minutes?: number; contentType?: string } = {}): LibrarySave => ({
        rkey: title,
        itemUrl: `https://example.com/${title}`,
        createdAt: '2026-01-01T00:00:00Z',
        title,
        ...extra,
    });

    test('reads split at five and fifteen minutes, with videos and unmeasured links after them', () => {
        const groups = queueGroups([
            save('link'),
            save('sixteen', { minutes: 16 }),
            save('five', { minutes: 5 }),
            save('video', { contentType: 'video' }),
            save('six', { minutes: 6 }),
            save('fifteen', { minutes: 15 }),
        ]);
        expect(groups.map((group) => [group.title, group.saves.map((s) => s.title)])).toEqual([
            ['Quick reads', ['five']],
            ['Over coffee', ['six', 'fifteen']],
            ['Deep dives', ['sixteen']],
            ['To watch', ['video']],
            ['Links', ['link']],
        ]);
    });
});
