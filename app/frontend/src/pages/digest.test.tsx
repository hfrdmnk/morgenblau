import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEvents = { Event, EventTarget };
GlobalRegistrator.register();
Object.assign(globalThis, nativeEvents);
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Digest } = await import('./digest');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
let cleanup = () => {};
afterEach(() => {
    act(() => cleanup());
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

type Routes = { feeds: Response; newsletters: Response };

async function emptyDigest({ feeds, newsletters }: Routes) {
    globalThis.fetch = Object.assign(async (input: RequestInfo | URL) => {
        const path = new URL(String(input), 'http://app.example.com').pathname;
        if (path === '/api/digest') return Response.json({ date: '2026-10-29', entries: [], hasActiveJob: false });
        if (path === '/api/subscriptions') return feeds.clone();
        if (path === '/api/newsletters') return newsletters.clone();
        return Response.json({ code: 'not_found', message: 'not found' }, { status: 404 });
    }, { preconnect: realFetch.preconnect });
    const container = document.createElement('div');
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Digest />));
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    return container.textContent ?? '';
}

const notFound = () => Response.json({ code: 'not_found', message: 'not found' }, { status: 404 });

test('a reader with no feeds gets the first-run state even when the server has no newsletter delivery', async () => {
    const text = await emptyDigest({ feeds: Response.json([]), newsletters: notFound() });
    expect(text).toContain('Nothing to read yet.');
    expect(text).not.toContain('A quiet day.');
});

test('a reader with only a newsletter gets the quiet-day state', async () => {
    const text = await emptyDigest({
        feeds: Response.json([]),
        newsletters: Response.json({ active: [{ id: 'n1', title: 'Example Publication', senderAddress: 'news@publication.example.com', primary: false, tags: [] }], stopped: [] }),
    });
    expect(text).toContain('A quiet day.');
});

test('a failed feed list offers a retry instead of guessing which empty state applies', async () => {
    const text = await emptyDigest({
        feeds: Response.json({ code: 'internal_error', message: 'internal error' }, { status: 500 }),
        newsletters: Response.json({ active: [], stopped: [] }),
    });
    expect(text).toContain('Couldn’t load your sources.');
    expect(text).not.toContain('A quiet day.');
    expect(text).not.toContain('Nothing to read yet.');
});
