import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { toast } from 'sonner';

GlobalRegistrator.register();
afterAll(() => GlobalRegistrator.unregister());

const { createEntrySave } = await import('./entry-save');
const { announceRemove } = await import('./save-announce');

test('a removal the server refuses takes back its Removed toast, so Undo is never offered for it', async () => {
    globalThis.fetch = (async () => Response.json({}, { status: 502 })) as unknown as typeof fetch;
    const save = createEntrySave({
        entrySlug: 'abc',
        title: 'Example',
        url: 'https://example.com/post',
        contentType: 'blogpost',
        publishedAt: '2026-10-30T00:00:00Z',
        body: null,
        source: { title: 'Example Publication', siteUrl: null, feedUrl: 'https://example.com/feed.xml' },
        savedState: { rkey: '3old', tags: ['design'], savedAt: '2026-11-20T08:00:00Z' },
    })!;

    expect(await announceRemove(save, ['design'])).toBe(false);

    expect(toast.getToasts()).not.toContainEqual(expect.objectContaining({ title: 'Removed from Library' }));
    expect(save.get()).toEqual({ saved: true, tags: ['design'], savedAt: '2026-11-20T08:00:00Z' });
});
