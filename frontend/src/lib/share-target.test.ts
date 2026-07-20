import { describe, expect, test } from 'bun:test';

import { shareTargetPresentation } from './share-target';

describe('shareTargetPresentation', () => {
    test('opens cached entries in the reader under their resolved title', () => {
        expect(
            shareTargetPresentation({
                title: 'A readable title',
                targetUrl: 'https://example.com/posts/readable',
                entrySlug: 'readable',
            }),
        ).toEqual({
            label: 'A readable title',
            href: '/entry/readable',
            external: false,
        });
    });

    test('opens uncached resolved items at their external target', () => {
        expect(
            shareTargetPresentation({
                title: 'From the Morgenblau lexicon',
                document:
                    'at://did:plc:publisher/site.standard.document/3example',
                targetUrl: 'https://publisher.example/writing/example',
            }),
        ).toEqual({
            label: 'From the Morgenblau lexicon',
            href: 'https://publisher.example/writing/example',
            external: true,
        });
    });

    test('uses a hostname when a title is unavailable', () => {
        expect(
            shareTargetPresentation({
                itemUrl: 'https://www.example.com/post',
            }),
        ).toEqual({
            label: 'example.com',
            href: 'https://www.example.com/post',
            external: true,
        });
    });

    test('never exposes AT-URIs or malformed targets as labels', () => {
        expect(
            shareTargetPresentation({
                document:
                    'at://did:plc:publisher/site.standard.document/3example',
            }),
        ).toEqual({
            label: 'Shared item',
            href: undefined,
            external: false,
        });
        expect(
            shareTargetPresentation({ itemUrl: 'not a valid URL' }),
        ).toEqual({
            label: 'Shared item',
            href: undefined,
            external: false,
        });
    });

    test('rejects identifier-shaped metadata titles', () => {
        expect(
            shareTargetPresentation({
                title: 'https://www.example.com/post',
                itemUrl: 'https://www.example.com/post',
            }).label,
        ).toBe('example.com');
        expect(
            shareTargetPresentation({
                title: 'at://did:plc:publisher/site.standard.document/3example',
                document:
                    'at://did:plc:publisher/site.standard.document/3example',
            }).label,
        ).toBe('Shared item');
    });

    test('keeps a pathless titled document as plain text', () => {
        expect(
            shareTargetPresentation({
                title: 'Publication note',
                document:
                    'at://did:plc:publisher/site.standard.document/3pathless',
            }),
        ).toEqual({
            label: 'Publication note',
            href: undefined,
            external: false,
        });
    });
});
