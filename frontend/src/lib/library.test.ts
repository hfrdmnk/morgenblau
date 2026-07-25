import { afterEach, describe, expect, test } from 'bun:test';

import {
    fetchNetworkShares,
    fetchSaves,
    uniqueSharerDIDs,
    type NetworkShare,
    type Save,
} from './library';
import { shareTargetPresentation, type ShareTarget } from './share-target';

const ALICE = 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa';
const BOB = 'did:plc:bbbbbbbbbbbbbbbbbbbbbbbb';

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

function share(overrides: Partial<NetworkShare> = {}): NetworkShare {
    return {
        sharerDid: ALICE,
        kind: 'rss',
        createdAt: '2026-07-01T00:00:00Z',
        ...overrides,
    };
}

describe('uniqueSharerDIDs', () => {
    test('dedupes repeated sharers while preserving first-seen order', () => {
        const shares = [
            share({ sharerDid: ALICE }),
            share({ sharerDid: BOB }),
            share({ sharerDid: ALICE }),
        ];
        expect(uniqueSharerDIDs(shares)).toEqual([ALICE, BOB]);
    });

    test('empty input yields an empty list', () => {
        expect(uniqueSharerDIDs([])).toEqual([]);
    });
});

describe('fetchNetworkShares', () => {
    test('returns the rows undecorated, with no profile lookup', async () => {
        const urls = stubJSON([share({ title: 'Example Article' })]);

        const shares = await fetchNetworkShares();

        expect(shares).toEqual([share({ title: 'Example Article' })]);
        expect(urls).toEqual(['/api/library/network-shares']);
    });

    test('reads a null body as no shares', async () => {
        stubJSON(null);
        expect(await fetchNetworkShares()).toEqual([]);
    });
});

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

    test('a save is usable as a share target', () => {
        const save: Save = {
            rkey: '3lasavealpha',
            itemUrl: 'https://news.example.com/posts/one',
            createdAt: '2026-07-01T00:00:00Z',
            title: 'Example Article',
        };
        const target: ShareTarget = save;

        expect(shareTargetPresentation(target)).toEqual({
            label: 'Example Article',
            href: 'https://news.example.com/posts/one',
            external: true,
        });
    });
});
