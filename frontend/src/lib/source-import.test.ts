import { afterEach, expect, test } from 'bun:test';

import { importSources } from './source-import';

const realFetch = globalThis.fetch;
afterEach(() => {
    globalThis.fetch = realFetch;
});

function stubFetch(
    implementation: (
        ...args: Parameters<typeof fetch>
    ) => ReturnType<typeof fetch>,
) {
    globalThis.fetch = Object.assign(implementation, {
        preconnect: realFetch.preconnect,
    });
}

const sources = Array.from({ length: 12 }, (_, i) => ({
    feedUrl: `https://example.com/feed/${i}`,
}));

test('imports sequential five-source batches and aggregates distinct outcomes', async () => {
    const lengths: number[] = [];
    const progress: number[] = [];
    stubFetch(async (_url, init) => {
        const batch = JSON.parse(String(init?.body)).sources;
        lengths.push(batch.length);
        return Response.json({
            added: batch.length - 2,
            updated: 1,
            unchanged: 1,
            failures: [],
        });
    });
    const result = await importSources(
        sources,
        new AbortController().signal,
        (done) => progress.push(done),
    );
    expect(lengths).toEqual([5, 5, 2]);
    expect(progress).toEqual([5, 10, 12]);
    expect(result).toEqual({
        added: 6,
        updated: 3,
        unchanged: 3,
        failures: [],
        remaining: [],
    });
});

test('partial failure retries only failed and unattempted sources', async () => {
    let calls = 0;
    stubFetch(async () => {
        calls++;
        return Response.json({
            added: 3,
            updated: 1,
            unchanged: 0,
            failures: [{ feedUrl: sources[2].feedUrl, message: 'Unavailable' }],
        });
    });
    const result = await importSources(
        sources,
        new AbortController().signal,
        () => {},
    );
    expect(calls).toBe(1);
    expect(result.remaining).toEqual([sources[2], ...sources.slice(5)]);
    expect(result.added).toBe(3);
    expect(result.updated).toBe(1);
});

test('lost batch response retains all uncertain sources without losing earlier success', async () => {
    let calls = 0;
    stubFetch(async () => {
        if (++calls === 2) throw new TypeError('Network error');
        return Response.json({
            added: 5,
            updated: 0,
            unchanged: 0,
            failures: [],
        });
    });
    const result = await importSources(
        sources,
        new AbortController().signal,
        () => {},
    );
    expect(calls).toBe(2);
    expect(result.added).toBe(5);
    expect(result.remaining).toEqual(sources.slice(5));
    expect(result.error).toContain('retry safely');
});
