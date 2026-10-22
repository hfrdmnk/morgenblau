import { describe, expect, test } from 'bun:test';

import { entryHref, entryListPath, newsletterSourceHref, readerSourceHref, sourceHref, sourceOrigin } from './paths';

describe('reader origin', () => {
    test('a source page round-trips through the entry link', () => {
        for (const origin of [sourceHref('3kexample2a'), newsletterSourceHref('nl_example')]) {
            const from = new URL(entryHref('slug', origin), 'https://app.example.com').searchParams.get('from');
            expect(sourceOrigin(from)).toBe(origin);
        }
    });

    test('anything that is not a source page is refused', () => {
        for (const from of [null, '2026-10-14', '/sources', '/library', '//evil.example/sources/x', 'https://evil.example/sources/x', '/sources/x/../../library', '/sources/x?y', '/sources/\\evil.example']) {
            expect(sourceOrigin(from)).toBeNull();
        }
    });
});

describe('reader source link', () => {
    test('a newsletter links by its private id, a feed by its rkey', () => {
        expect(readerSourceHref({ kind: 'newsletter', id: 'nl_example', rkey: 'ignored' })).toBe('/sources/newsletters/nl_example');
        expect(readerSourceHref({ rkey: '3kexample2a' })).toBe('/sources/3kexample2a');
        expect(readerSourceHref({ kind: 'newsletter' })).toBeNull();
    });
});

describe('entry list', () => {
    test('an entry steps through the list it was opened from', () => {
        expect(entryListPath(sourceHref('3kexample2a'), 'Europe/Berlin')).toBe('/api/subscriptions/3kexample2a/entries');
        expect(entryListPath(newsletterSourceHref('nl_example'), 'Europe/Berlin')).toBe('/api/newsletters/nl_example/entries');
        expect(entryListPath('2026-10-14', 'Europe/Berlin')).toBe('/api/digest?date=2026-10-14&timezone=Europe%2FBerlin');
    });

    test('an entry without a trusted origin has no list', () => {
        for (const from of [null, '', '2026-02-30', '/library', '/sources']) {
            expect(entryListPath(from, 'Europe/Berlin')).toBeNull();
        }
    });
});
