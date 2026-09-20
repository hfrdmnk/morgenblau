import { afterEach, describe, expect, test } from 'bun:test';

import {
    fetchSaves,
    savePresentation,
    type Save,
} from './library';

const realFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = realFetch;
});

// Records every URL requested so a test can pin the endpoint it went to.
function stubJSON(body: unknown): string[] {
    const urls: string[] = [];
    globalThis.fetch = (async (url: string | URL) => {
        urls.push(String(url));
        return new Response(JSON.stringify(body), { status: 200 });
    }) as typeof fetch;
    return urls;
}

describe('fetchSaves', () => {
    test('returns the saved items', async () => {
        const urls = stubJSON([
            {
                rkey: '3lasavealpha',
                itemUrl: 'https://news.example.com/posts/one',
                createdAt: '2026-07-01T00:00:00Z',
                title: 'Example Article',
            },
        ]);

        const saves = await fetchSaves();

        expect(saves).toHaveLength(1);
        expect(saves[0]?.rkey).toBe('3lasavealpha');
        expect(urls).toEqual(['/api/saves']);
    });

    test('reads a null body as no saves', async () => {
        stubJSON(null);
        expect(await fetchSaves()).toEqual([]);
    });

    test('opens a cached save in the reader', () => {
        const save: Save = {
            rkey: '3lasavealpha',
            itemUrl: 'https://news.example.com/posts/one',
            createdAt: '2026-07-01T00:00:00Z',
            title: 'Example Article',
            entrySlug: 'one',
        };

        expect(savePresentation(save)).toEqual({
            label: 'Example Article',
            href: '/entry/one',
            external: false,
        });
    });

    test('opens a private newsletter save without a public URL', () => {
        const save: Save = {
            kind: 'newsletter',
            id: 'save-private-one',
            createdAt: '2026-07-01T00:00:00Z',
            title: 'A private issue',
            entrySlug: 'newsletter-one',
        };

        expect(savePresentation(save)).toEqual({
            label: 'A private issue',
            href: '/entry/newsletter-one',
            external: false,
        });
    });

    test('falls back to the hostname for an uncached save', () => {
        expect(
            savePresentation({
                rkey: '3lasavealpha',
                itemUrl: 'https://news.example.com/posts/one',
                createdAt: '2026-07-01T00:00:00Z',
            }),
        ).toEqual({
            label: 'news.example.com',
            href: 'https://news.example.com/posts/one',
            external: true,
        });
    });

    test('does not render identifier-shaped titles or unsafe links', () => {
        expect(
            savePresentation({
                rkey: '3lasavealpha',
                itemUrl: 'javascript:alert(1)',
                createdAt: '2026-07-01T00:00:00Z',
                title: 'at://did:plc:publisher/site.standard.document/3example',
            }),
        ).toEqual({
            label: 'Saved item',
            href: undefined,
            external: false,
        });
    });
});
