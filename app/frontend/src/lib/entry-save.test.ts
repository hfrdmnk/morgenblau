import { afterEach, expect, test } from 'bun:test';

import { createEntrySave } from './entry-save';
import type { ReaderEntry } from './reader';

const realFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = realFetch; });

const entry: ReaderEntry = {
    entrySlug: 'abc', title: 'Example', url: 'https://example.com/post', contentType: 'blogpost',
    publishedAt: '2026-10-30T00:00:00Z', body: null,
    source: { title: 'Example Publication', siteUrl: null, feedUrl: 'https://example.com/feed.xml' },
};

function stubFetch(respond: (method: string) => Promise<Response> | Response): string[] {
    const calls: string[] = [];
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
        calls.push(`${init?.method} ${String(url)} ${init?.body ?? ''}`);
        return respond(init?.method ?? 'GET');
    }) as typeof fetch;
    return calls;
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test('a tag added while the save is in flight patches the record that save creates', async () => {
    let created!: () => void;
    const calls = stubFetch(async (method) => {
        if (method !== 'POST') return Response.json({ tags: ['read-later'] });
        await new Promise<void>((resolve) => { created = resolve; });
        return Response.json({ rkey: '3new' });
    });

    const save = createEntrySave(entry)!;
    save.save();
    save.setTags(['read-later']);
    expect(save.get()).toEqual({ saved: true, tags: ['read-later'] });
    await settle();
    created();
    await settle();

    expect(calls).toEqual([
        'POST /api/saves {"itemUrl":"https://example.com/post","feedUrl":"https://example.com/feed.xml"}',
        'PATCH /api/saves/3new {"tags":["read-later"]}',
    ]);
});

test('a failed tag change falls back to what the server confirmed', async () => {
    stubFetch(() => Response.json({}, { status: 502 }));
    const save = createEntrySave({ ...entry, savedState: { rkey: '3old', tags: ['design'] } })!;
    save.setTags(['design', 'weekend']);
    await settle();
    expect(save.get()).toEqual({ saved: true, tags: ['design'] });
});

test('a failure drops the changes queued behind it, so the server keeps what the reader sees', async () => {
    let failed = false;
    const calls = stubFetch(() => {
        if (failed) return Response.json({ tags: ['design', 'weekend', 'later'] });
        failed = true;
        return Response.json({}, { status: 502 });
    });
    const save = createEntrySave({ ...entry, savedState: { rkey: '3old', tags: ['design'] } })!;
    save.setTags(['design', 'weekend']);
    save.setTags(['design', 'weekend', 'later']);
    for (let i = 0; i < 5; i++) await settle();

    expect(calls).toEqual(['PATCH /api/saves/3old {"tags":["design","weekend"]}']);
    expect(save.get()).toEqual({ saved: true, tags: ['design'] });
});
