import { afterEach, describe, expect, test } from 'bun:test';

import { chunked, fetchProfiles, type Profile } from './profile';

const ALICE = 'did:plc:aaaaaaaaaaaaaaaaaaaaaaaa';
const BOB = 'did:plc:bbbbbbbbbbbbbbbbbbbbbbbb';

const realFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = realFetch;
});

function did(n: number): string {
    return `did:plc:${String(n).padStart(24, 'a')}`;
}

// Replays the requested DIDs to the handler and records every URL the batch fetcher hit.
function stubProfiles(handler: (dids: string[]) => Response): string[] {
    const urls: string[] = [];
    globalThis.fetch = (async (url: string | URL) => {
        const raw = String(url);
        urls.push(raw);
        const dids =
            new URL(raw, 'https://app.example').searchParams.get('dids') ?? '';
        return handler(dids === '' ? [] : dids.split(','));
    }) as typeof fetch;
    return urls;
}

function found(dids: string[]): Response {
    const profiles: Record<string, Profile> = Object.fromEntries(
        dids.map((d) => [d, { did: d, handle: `${d.slice(-4)}.example` }]),
    );
    return new Response(JSON.stringify({ profiles }), { status: 200 });
}

describe('chunked', () => {
    test('splits into runs of the requested size', () => {
        expect(chunked([1, 2, 3, 4, 5, 6], 2)).toEqual([
            [1, 2],
            [3, 4],
            [5, 6],
        ]);
    });

    test('keeps the trailing partial chunk', () => {
        expect(chunked([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
    });

    test('yields a single chunk when the size exceeds the input', () => {
        expect(chunked(['a', 'b'], 50)).toEqual([['a', 'b']]);
    });

    test('empty input yields no chunks', () => {
        expect(chunked([], 50)).toEqual([]);
    });
});

describe('fetchProfiles', () => {
    test('resolves a small list in one batch request', async () => {
        const urls = stubProfiles(found);

        const profiles = await fetchProfiles([ALICE, BOB]);

        expect(urls).toHaveLength(1);
        expect(profiles[ALICE]?.did).toBe(ALICE);
        expect(profiles[BOB]?.did).toBe(BOB);
    });

    test('encodes each DID and joins them with commas', async () => {
        const urls = stubProfiles(found);

        await fetchProfiles([ALICE, BOB]);

        expect(urls[0]).toBe(
            `/api/profiles?dids=${encodeURIComponent(ALICE)},${encodeURIComponent(BOB)}`,
        );
    });

    test('dedupes repeated DIDs before requesting', async () => {
        let requested: string[] = [];
        stubProfiles((dids) => {
            requested = dids;
            return found(dids);
        });

        await fetchProfiles([ALICE, BOB, ALICE]);

        expect(requested).toEqual([ALICE, BOB]);
    });

    test('splits past the 50-DID server cap into batches', async () => {
        const sizes: number[] = [];
        stubProfiles((dids) => {
            sizes.push(dids.length);
            return found(dids);
        });

        const dids = Array.from({ length: 120 }, (_, i) => did(i));
        const profiles = await fetchProfiles(dids);

        expect(sizes).toEqual([50, 50, 20]);
        expect(Object.keys(profiles)).toHaveLength(120);
        expect(profiles[did(119)]?.did).toBe(did(119));
    });

    test('keeps the profiles from batches that succeeded when one fails', async () => {
        const dids = Array.from({ length: 60 }, (_, i) => did(i));
        stubProfiles((batch) =>
            batch.includes(did(0))
                ? new Response(null, { status: 500 })
                : found(batch),
        );

        const profiles = await fetchProfiles(dids);

        expect(profiles[did(0)]).toBeUndefined();
        expect(profiles[did(55)]?.did).toBe(did(55));
    });

    test('reads a null entry as an unresolved profile', async () => {
        stubProfiles(() =>
            new Response(JSON.stringify({ profiles: { [ALICE]: null } }), {
                status: 200,
            }),
        );

        const profiles = await fetchProfiles([ALICE]);

        expect(ALICE in profiles).toBe(true);
        expect(profiles[ALICE]).toBeUndefined();
    });

    test('makes no request for an empty DID list', async () => {
        const urls = stubProfiles(found);

        expect(await fetchProfiles([])).toEqual({});
        expect(urls).toEqual([]);
    });
});
