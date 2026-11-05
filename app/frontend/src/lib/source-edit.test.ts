import { afterEach, expect, test } from 'bun:test';

import { addTags, canAddTags, hasOverlongTag, editNewsletter, removeSource, resumeNewsletter, titleTooLong, unsubscribeNewsletter, deleteNewsletter, type EditableSource } from './source-edit';

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
    kind: 'newsletter', id: 'nl-1', title: 'Example Letter', tags: ['weekly', 'design'], stopped: false, canUnsubscribe: true,
} as const satisfies EditableSource;

test('a newsletter edit patches only the fields that changed', async () => {
    record();
    await editNewsletter(newsletter, { title: 'Example Renamed' });
    await editNewsletter(newsletter, { tags: [], primary: true });
    expect(requests).toEqual([
        { url: '/api/newsletters/nl-1', method: 'PATCH', body: { title: 'Example Renamed' } },
        { url: '/api/newsletters/nl-1', method: 'PATCH', body: { tags: [], primary: true } },
    ]);
});

test('removing deletes a feed but stops a newsletter, which unsubscribing or deleting removes', async () => {
    record(204);
    await removeSource(feed);
    await removeSource(newsletter);
    await resumeNewsletter({ ...newsletter, stopped: true });
    await unsubscribeNewsletter(newsletter);
    await deleteNewsletter(newsletter);
    expect(requests.map(({ method, url }) => `${method} ${url}`)).toEqual([
        'DELETE /api/subscriptions/3kexample',
        'POST /api/newsletters/nl-1/stop',
        'POST /api/newsletters/nl-1/enable',
        'POST /api/newsletters/nl-1/unsubscribe',
        'DELETE /api/newsletters/nl-1',
    ]);
});

test('ids are encoded into the path', async () => {
    record();
    await removeSource({ ...feed, rkey: 'a/b' });
    await editNewsletter({ ...newsletter, id: 'n/1' }, { primary: false });
    expect(requests.map(({ url }) => url)).toEqual(['/api/subscriptions/a%2Fb', '/api/newsletters/n%2F1']);
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
