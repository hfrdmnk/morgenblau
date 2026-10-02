import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

import type { ReaderEntry } from '@/lib/reader';

// React DOM reads the environment when it loads, so the DOM must exist before the dynamic imports below.
GlobalRegistrator.register();
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { NewsletterImages } = await import('./newsletter-images');

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
const requests: { url: string; method: string }[] = [];
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    requests.length = 0;
    globalThis.fetch = realFetch;
});

afterAll(() => GlobalRegistrator.unregister());

function newsletterEntry(newsletter: Partial<NonNullable<ReaderEntry['newsletter']>>): ReaderEntry {
    return {
        entrySlug: 'example-issue',
        title: 'Example issue',
        contentType: 'newsletter',
        publishedAt: '2026-09-30T09:00:00Z',
        body: '<p>Example body</p>',
        source: { title: 'Example Letters', siteUrl: null },
        newsletter: {
            messageId: 'message-1',
            senderName: 'Example Letters',
            senderAddress: 'hello@letters.example.com',
            sentAt: null,
            hasBlockedRemoteImages: true,
            remoteImagesAllowed: false,
            ...newsletter,
        },
    };
}

async function render(entry: ReaderEntry, onChange: (entry: ReaderEntry) => void = () => {}) {
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
        requests.push({ url: String(url), method: init?.method ?? 'GET' });
        return Response.json({ body: '<img src="https://images.example.com/cover.png">', remoteImagesAllowed: true, hasBlockedRemoteImages: false });
    }) as typeof fetch;
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () => root.render(<NewsletterImages entry={entry} newsletter={entry.newsletter!} onChange={onChange} />));
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    return container;
}

test('blocked newsletter images load only after the reader clicks', async () => {
    let changed: ReaderEntry | undefined;
    const container = await render(newsletterEntry({}), (entry) => { changed = entry; });

    expect(container.textContent).toContain('Remote images are blocked');
    expect(requests).toEqual([]);

    await act(async () => container.querySelector('button')!.click());

    expect(requests).toEqual([{ url: '/api/newsletters/messages/message-1/images', method: 'POST' }]);
    expect(changed?.newsletter?.remoteImagesAllowed).toBe(true);
});

test('no banner or request when nothing is blocked or images are already allowed', async () => {
    for (const newsletter of [{ hasBlockedRemoteImages: false }, { remoteImagesAllowed: true }]) {
        const container = await render(newsletterEntry(newsletter));
        expect(container.textContent).not.toContain('Remote images are blocked');
        expect(requests).toEqual([]);
        act(() => unmount());
    }
});
