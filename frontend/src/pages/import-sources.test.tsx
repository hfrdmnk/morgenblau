import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

import type { FeedSource } from '@/hooks/use-sources';
import type { ImportSource } from '@/lib/source-import';

const nativeEvents = { Event, EventTarget };
GlobalRegistrator.register();
Object.assign(globalThis, nativeEvents);
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Router } = await import('wouter');
const { memoryLocation } = await import('wouter/memory-location');
const { ImportSources } = await import('./import-sources');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
let cleanup = () => {};
afterEach(() => {
    act(() => cleanup());
    cleanup = () => {};
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

const [healthy, firstUnavailable, secondUnavailable, waiting, failed, unattempted, unrelated] = [
    { feedUrl: 'https://healthy.example.com/feed.xml', title: 'Healthy Example Publication' },
    { feedUrl: 'https://first.example.com/feed.xml', title: 'First Unavailable Example Publication' },
    { feedUrl: 'https://second.example.com/feed.xml', title: 'Second Unavailable Example Publication' },
    { feedUrl: 'https://waiting.example.com/feed.xml', title: 'Unchecked Example Publication' },
    { feedUrl: 'https://failed.example.com/feed.xml', title: 'Failed Example Import' },
    { feedUrl: 'https://unattempted.example.com/feed.xml', title: 'Unattempted Example Import' },
    { feedUrl: 'https://unrelated.example.com/feed.xml', title: 'Unrelated Example Publication' },
];

function feed(source: ImportSource, fetchStatus: FeedSource['fetchStatus']): FeedSource {
    return { ...source, rkey: source.feedUrl, primary: false, fetchStatus };
}

async function click(name: string) {
    const control = [...document.querySelectorAll<HTMLElement>('button, [role="button"]')]
        .find((element) => element.textContent?.trim() === name);
    expect(control).toBeDefined();
    await act(async () => control!.click());
}

async function importReceipt(sources: ImportSource[], feeds: FeedSource[], failedSources: ImportSource[] = []) {
    const batches: ImportSource[][] = [];
    globalThis.fetch = Object.assign(async (url: Parameters<typeof fetch>[0], init?: RequestInit) => {
        switch (String(url)) {
            case '/api/subscriptions/import/prepare':
                return Response.json({ sources, warnings: [] });
            case '/api/subscriptions/import': {
                const batch: ImportSource[] = JSON.parse(String(init?.body)).sources;
                batches.push(batch);
                return Response.json({
                    added: batch.length - failedSources.length, updated: 0, unchanged: 0,
                    failures: failedSources.map((source) => ({ feedUrl: source.feedUrl, message: 'Subscription could not be saved.' })),
                });
            }
            case '/api/subscriptions':
                return Response.json(feeds);
            default:
                throw new Error(`Unexpected request: ${String(url)}`);
        }
    }, { preconnect: realFetch.preconnect });
    const location = memoryLocation({ path: '/settings/import', record: true });
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Router hook={location.hook}><ImportSources /></Router>));
    await click('Import from Skyreader');
    await click('Import sources');
    expect(document.querySelector('[role="dialog"]')).not.toBeNull();
    return { container, location, batches };
}

test('the rendered receipt lists only unavailable confirmed imports, in import order', async () => {
    const { batches } = await importReceipt(
        [healthy, firstUnavailable, secondUnavailable, waiting, failed, unattempted],
        [feed(failed, 'unavailable'), feed(secondUnavailable, 'unavailable'), feed(unrelated, 'unavailable'),
            feed(healthy, 'ready'), feed(waiting, 'waiting'), feed(firstUnavailable, 'unavailable'), feed(unattempted, 'unavailable')],
        [failed],
    );
    const dialog = document.querySelector('[role="dialog"]')!;
    expect(dialog.textContent).toContain('Import paused');
    expect(dialog.textContent).toContain('4 added, 0 updated, 0 already up to date.');
    expect(batches).toEqual([[healthy, firstUnavailable, secondUnavailable, waiting, failed]]);
    await click('Posts not fetching for 2 sources');
    const panel = dialog.querySelector('[data-slot="accordion-content"]')!;
    expect([...panel.querySelectorAll('li')].map((item) => item.textContent)).toEqual([
        'First Unavailable Example Publication', 'Second Unavailable Example Publication',
    ]);
    expect(dialog.textContent).toContain('Retry remaining sources');
});

test('a single unavailable import has singular wording, and View sources closes and navigates', async () => {
    const { container, location } = await importReceipt(
        [firstUnavailable], [feed(unrelated, 'unavailable'), feed(firstUnavailable, 'unavailable')],
    );
    expect(document.querySelector('[role="dialog"]')!.textContent).toContain('Subscriptions saved');
    await click('Posts not fetching for 1 source');
    expect([...document.querySelectorAll('[data-slot="accordion-content"] li')].map((item) => item.textContent))
        .toEqual(['First Unavailable Example Publication']);
    await click('View sources');
    expect(location.history).toEqual(['/settings/import', '/sources']);
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(container.querySelector('h1')?.textContent).toBe('Import sources');
});

test('a healthy-only receipt has no warning even when other subscriptions are unavailable', async () => {
    await importReceipt([healthy], [feed(unrelated, 'unavailable'), feed(healthy, 'ready')]);
    const dialog = document.querySelector('[role="dialog"]')!;
    expect(dialog.textContent).toContain('Subscriptions saved');
    expect(dialog.textContent).toContain('1 added, 0 updated, 0 already up to date.');
    expect(dialog.querySelector('[data-slot="accordion-trigger"]')).toBeNull();
    expect(dialog.textContent).not.toContain('Checking posts');
    await click('Done');
    expect(document.querySelector('[role="dialog"]')).toBeNull();
});
