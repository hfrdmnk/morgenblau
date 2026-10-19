import { afterEach, expect, test } from 'bun:test';

import { addTags, canAddTags, hasOverlongTag, removeSource, renameSource, resumeNewsletter, retagSource, titleTooLong, type EditableSource } from './source-edit';

const realFetch = globalThis.fetch;
let requests: { url: string; method: string; body: unknown }[] = [];

function record(status = 200) {
    requests = [];
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: RequestInit) => {
            requests.push({ url: String(url), method: init?.method ?? 'GET', body: init?.body ? JSON.parse(String(init.body)) : undefined });
            return status === 204 ? new Response(null, { status }) : Response.json({}, { status });
        },
        { preconnect: realFetch.preconnect },
    );
}

afterEach(() => {
    globalThis.fetch = realFetch;
});

const feed: EditableSource = { kind: 'feed', rkey: '3kexample', title: 'Example Publication', tags: ['tech'] };
const newsletter = {
    kind: 'newsletter', id: 'nl-1', title: 'Example Letter', tags: ['weekly', 'design'], stopped: false,
} as const satisfies EditableSource;

test('an edit patches only the field that changed, for feeds and newsletters alike', async () => {
    record();
    await renameSource(feed, '  Example Renamed  ');
    await retagSource(feed, ['tech', 'news']);
    await renameSource(newsletter, 'Example Renamed');
    await retagSource(newsletter, []);
    expect(requests).toEqual([
        { url: '/api/subscriptions/3kexample', method: 'PATCH', body: { title: 'Example Renamed' } },
        { url: '/api/subscriptions/3kexample', method: 'PATCH', body: { tags: ['tech', 'news'] } },
        { url: '/api/newsletters/nl-1', method: 'PATCH', body: { title: 'Example Renamed' } },
        { url: '/api/newsletters/nl-1', method: 'PATCH', body: { tags: [] } },
    ]);
});

test('removing deletes a feed but stops or resumes a newsletter, which v1 never deletes', async () => {
    record(204);
    await removeSource(feed);
    await removeSource(newsletter);
    await resumeNewsletter({ ...newsletter, stopped: true });
    expect(requests.map(({ method, url }) => `${method} ${url}`)).toEqual([
        'DELETE /api/subscriptions/3kexample',
        'POST /api/newsletters/nl-1/stop',
        'POST /api/newsletters/nl-1/enable',
    ]);
});

test('ids are encoded into the path', async () => {
    record();
    await renameSource({ ...feed, rkey: 'a/b' }, 'x');
    expect(requests[0].url).toBe('/api/subscriptions/a%2Fb');
});

test('adding tags splits on commas, trims, and skips case-insensitive duplicates', () => {
    expect(addTags(['Tech'], ' news, tech ,, Design ')).toEqual(['Tech', 'news', 'Design']);
    expect(addTags(['Tech'], '   ')).toEqual(['Tech']);
});

test('length limits count characters, not bytes or UTF-16 units', () => {
    expect(hasOverlongTag(`ok, ${'é'.repeat(64)}`)).toBe(false);
    expect(hasOverlongTag(`ok, ${'a'.repeat(65)} `)).toBe(true);
    expect(hasOverlongTag('🌍'.repeat(64))).toBe(false);
    expect(titleTooLong(` ${'x'.repeat(128)} `)).toBe(false);
    expect(titleTooLong('x'.repeat(129))).toBe(true);
});

test('adding tags stops at the record limit of ten', () => {
    const nine = Array.from({ length: 9 }, (_, i) => `tag${i}`);
    expect(addTags(nine, 'ten, eleven')).toEqual([...nine, 'ten']);
    expect(canAddTags(nine, 'ten')).toBe(true);
    expect(canAddTags([...nine, 'ten'], 'eleven')).toBe(false);
    expect(canAddTags(nine, `ok, ${'a'.repeat(65)}`)).toBe(false);
});
