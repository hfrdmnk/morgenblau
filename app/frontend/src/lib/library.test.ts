import { describe, expect, test } from 'bun:test';

import { groupSaves, librarySort, libraryView, matchesQuery, savedPhrase, type LibrarySave } from '@/lib/library';

const save = (title: string, createdAt: string, source?: string): LibrarySave => ({
    rkey: `${title}-${createdAt}`,
    itemUrl: `https://${source ?? 'example.com'}/${encodeURIComponent(title)}`,
    createdAt,
    title,
});

const titles = (groups: ReturnType<typeof groupSaves>) =>
    groups.map((group) => [group.title, group.saves.map((s) => s.title || 'Untitled')]);

describe('Library view and sort', () => {
    test('the view and sort come from the URL and fall back to title', () => {
        expect(libraryView('?view=everything')).toBe('everything');
        expect(libraryView('?tab=archived')).toBe('recent');
        expect(libraryView('')).toBe('recent');
        expect(librarySort('?view=everything&sort=site')).toBe('site');
        expect(librarySort('?sort=time')).toBe('time');
        expect(librarySort('?sort=newest')).toBe('title');
        expect(librarySort('')).toBe('title');
    });
});

describe('grouping everything', () => {
    const now = new Date(2026, 10, 27, 12);
    const saves = [
        save('zebra crossings', new Date(2026, 10, 20).toISOString(), 'b.example.com'),
        save('Árbol', new Date(2026, 9, 3).toISOString(), 'a.example.com'),
        save('apples', new Date(2026, 0, 15).toISOString(), 'b.example.com'),
        save('', new Date(2025, 11, 30).toISOString(), 'a.example.com'),
    ];

    test('time keeps the saved order and names the year only for earlier years', () => {
        expect(titles(groupSaves(saves, 'time', now))).toEqual([
            ['November', ['zebra crossings']],
            ['October', ['Árbol']],
            ['January', ['apples']],
            ['December 2025', ['Untitled']],
        ]);
    });

    test('title ignores case and accents, and an untitled save files under U', () => {
        expect(titles(groupSaves(saves, 'title', now))).toEqual([
            ['A', ['apples', 'Árbol']],
            ['U', ['Untitled']],
            ['Z', ['zebra crossings']],
        ]);
    });

    test('site groups by source and keeps each site’s saves newest first', () => {
        expect(titles(groupSaves(saves, 'site', now))).toEqual([
            ['a.example.com', ['Árbol', 'Untitled']],
            ['b.example.com', ['zebra crossings', 'apples']],
        ]);
    });
});

describe('saved phrase', () => {
    const now = new Date(2026, 10, 27, 9);
    test('reads as a sentence on either side of each boundary', () => {
        expect(savedPhrase(new Date(2026, 10, 27, 0, 5).toISOString(), now)).toBe('Saved to your Library today');
        expect(savedPhrase(new Date(2026, 10, 26, 23, 55).toISOString(), now)).toBe('Saved to your Library yesterday');
        expect(savedPhrase(new Date(2026, 10, 21, 12).toISOString(), now)).toBe('Saved to your Library 6 days ago');
        expect(savedPhrase(new Date(2026, 10, 20, 12).toISOString(), now)).toMatch(/^Saved to your Library on \D*20/);
    });
});

describe('Library search', () => {
    const zurich: LibrarySave = {
        rkey: 'z',
        itemUrl: 'https://www.notes.example.com/zurich',
        createdAt: '2026-11-01T00:00:00Z',
        title: 'Grüße aus Zürich',
        tags: ['Design-notes', 'travel'],
        source: { title: 'Example Travel Journal' },
    };
    const designed: LibrarySave = { ...zurich, rkey: 'd', title: 'Why design matters', tags: ['design'] };

    test('every word must match, ignoring case and accents, in the title, source, host or tags', () => {
        expect(matchesQuery(zurich, 'GRÜßE AUS')).toBe(true);
        expect(matchesQuery(zurich, 'zurich journal')).toBe(true);
        expect(matchesQuery(zurich, 'zurich weekly')).toBe(false);
        expect(matchesQuery(zurich, 'notes.example')).toBe(true);
        expect(matchesQuery(zurich, 'design')).toBe(true);
        expect(matchesQuery(zurich, '  ')).toBe(true);
    });

    test('#tag matches that tag exactly, not a word in the title or a longer tag', () => {
        expect(matchesQuery(designed, '#design')).toBe(true);
        expect(matchesQuery(zurich, '#design')).toBe(false);
        expect(matchesQuery({ ...designed, tags: [] }, '#design')).toBe(false);
        expect(matchesQuery(zurich, '#TRAVEL zürich')).toBe(true);
        expect(matchesQuery(zurich, '#')).toBe(true);
    });
});
