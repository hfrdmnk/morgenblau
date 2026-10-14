import { describe, expect, test } from 'bun:test';

import { entryHref, newsletterSourceHref, readerSourceHref, sourceHref, sourceOrigin } from './paths';

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
