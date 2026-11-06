import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEvents = { Event, EventTarget };
GlobalRegistrator.register({ url: 'http://app.example.com/' });
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

async function renderDigest(entries: unknown[], respond = () => Response.json({ date: '2026-10-29', entries, hasActiveJob: false })) {
    globalThis.fetch = Object.assign(async () => respond(), { preconnect: realFetch.preconnect });
    const container = document.createElement('div');
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Digest />));
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    return container;
}

function feedEntry(slug: string, title: string, primary: boolean, extra: Record<string, unknown> = {}) {
    return {
        id: slug, entrySlug: slug, title, url: `https://publication.example.com/posts/${slug}`, contentType: 'blogpost', publishedAt: '2026-10-29T08:00:00Z',
        body: null, source: { title: `${title} source`, siteUrl: 'https://publication.example.com', faviconUrl: null, primary }, ...extra,
    };
}

test('primary sources lead the digest with an excerpt, and an image only when the server offers one', async () => {
    const container = await renderDigest([
        feedEntry('newer', 'Newer ordinary post', false),
        feedEntry('pictured', 'Primary post with image', true, {
            body: '<p>First   paragraph.</p><p>Second <em>paragraph</em>.</p>',
            image: '/api/entries/pictured/image',
        }),
        feedEntry('plain', 'Primary post without image', true),
    ]);
    const titles = [...container.querySelectorAll('h2, li a[href*="newer"]')].map((node) => node.textContent);
    expect(titles).toEqual(['Primary post with image', 'Primary post without image', 'Newer ordinary post']);
    expect(container.textContent).toContain('First paragraph. Second paragraph.');
    const images = [...container.querySelectorAll('img')].map((img) => img.getAttribute('src'));
    expect(images).toEqual(['/api/entries/pictured/image']);
});

test('a source with several posts keeps its latest in place and folds the rest beneath it', async () => {
    const linkblog = { title: 'Example Linkblog', siteUrl: 'https://links.example.com', feedUrl: 'https://links.example.com/rss', faviconUrl: null };
    const post = (slug: string, source: Record<string, unknown>) => ({ ...feedEntry(slug, slug, false), source });
    const container = await renderDigest([
        post('links-latest', linkblog),
        post('weekly', { title: 'Example Weekly', siteUrl: 'https://links.example.com', feedUrl: 'https://weekly.example.com/feed', faviconUrl: null }),
        post('links-older', linkblog),
        post('letter', { kind: 'newsletter', id: 'letters', title: 'Example Letters', siteUrl: null, faviconUrl: null }),
        post('links-oldest', linkblog),
    ]);
    const visible = () => [...container.querySelectorAll('a[href*="?from="]')].map((link) => link.textContent);
    expect(visible()).toEqual(['links-latest', 'weekly', 'letter']);

    const toggle = container.querySelector<HTMLButtonElement>('button[aria-expanded]');
    expect(toggle?.textContent).toBe('+2 more');
    await act(async () => toggle?.click());
    // Unmounting mid-animation leaves the next test's digest stuck loading under happy-dom.
    await act(async () => new Promise((resolve) => setTimeout(resolve, 300)));
    expect(visible()).toEqual(['links-latest', 'links-older', 'links-oldest', 'weekly', 'letter']);
});

test('an open group stays open when a sync brings a newer post from its source, and when the page mounts again', async () => {
    const source = { title: 'Example Notes', siteUrl: 'https://notes.example.com', feedUrl: 'https://notes.example.com/feed', faviconUrl: null };
    const post = (slug: string) => ({ ...feedEntry(slug, slug, false), source });
    let syncing = true;
    const respond = () => Response.json({
        date: '2026-10-29',
        entries: syncing ? [post('notes-a'), post('notes-b')] : [post('notes-new'), post('notes-a'), post('notes-b')],
        hasActiveJob: syncing,
    });
    let container = await renderDigest([], respond);
    const visible = () => [...container.querySelectorAll('a[href*="?from="]')].map((link) => link.textContent);
    const settle = (ms: number) => act(async () => new Promise((resolve) => setTimeout(resolve, ms)));

    await act(async () => container.querySelector<HTMLButtonElement>('button[aria-expanded]')?.click());
    await settle(300);
    syncing = false;
    await settle(1200);
    expect(visible()).toEqual(['notes-new', 'notes-a', 'notes-b']);

    act(() => cleanup());
    container = await renderDigest([], respond);
    expect(visible()).toEqual(['notes-new', 'notes-a', 'notes-b']);
});

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

test('an empty day keeps the digest skeleton while a sync is still collecting, then shows what arrived', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(async () => {
        calls += 1;
        return Response.json(calls === 1
            ? { date: '2026-10-29', entries: [], hasActiveJob: true }
            : { date: '2026-10-29', entries: [feedEntry('arrived', 'Freshly collected post', false)], hasActiveJob: false });
    }, { preconnect: realFetch.preconnect });
    const container = document.createElement('div');
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Digest />));
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(calls).toBe(1);
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(container.textContent).not.toContain('A quiet day.');
    expect(container.textContent).not.toContain('Nothing to read yet.');

    await act(async () => new Promise((resolve) => setTimeout(resolve, 1100)));
    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
    expect(container.textContent).toContain('Freshly collected post');
});

test('adding a first source never flashes the quiet-day copy before the digest reports its collection', async () => {
    let feeds: unknown[] = [];
    let digestCalls = 0;
    let answerDigest = () => {};
    globalThis.fetch = Object.assign(async (input: RequestInfo | URL) => {
        const path = new URL(String(input), 'http://app.example.com').pathname;
        if (path === '/api/subscriptions') return Response.json(feeds);
        if (path === '/api/newsletters') return Response.json({ active: [], stopped: [] });
        digestCalls += 1;
        if (digestCalls === 1) return Response.json({ date: '2026-10-29', entries: [], hasActiveJob: false });
        await new Promise<void>((resolve) => { answerDigest = resolve; });
        return Response.json({ date: '2026-10-29', entries: [], hasActiveJob: true });
    }, { preconnect: realFetch.preconnect });
    const { subscriptionChanges } = await import('@/lib/add-source');
    const container = document.createElement('div');
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Digest />));
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(container.textContent).toContain('Nothing to read yet.');

    feeds = [feedEntry('first', 'Example Publication', false)];
    await act(async () => { subscriptionChanges.dispatchEvent(new Event('change')); });
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(container.textContent).not.toContain('A quiet day.');

    await act(async () => { answerDigest(); });
    await act(async () => new Promise((resolve) => setTimeout(resolve, 20)));
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
});
