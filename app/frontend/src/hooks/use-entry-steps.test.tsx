import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, spyOn, test } from 'bun:test';
import { toast } from 'sonner';

GlobalRegistrator.register({ url: 'http://app.example.com/' });
const notices: string[] = [];
const info = spyOn(toast, 'info').mockImplementation((message) => notices.push(String(message)));
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Route, Router, useParams } = await import('wouter');
const { useSearch } = await import('wouter/use-browser-location');
const { useEntrySteps } = await import('./use-entry-steps');
const { useAppLocation } = await import('./use-app-location');

// Wouter's module may be cached from a different Happy DOM window.
for (const method of ['pushState', 'replaceState'] as const) {
    const original = window.history[method].bind(window.history);
    window.history[method] = (...args: Parameters<History[typeof method]>) => {
        original(args[0], args[1], args[2]);
        window.dispatchEvent(new Event('popstate'));
    };
}

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
let unmount = () => {};
afterEach(() => {
    act(() => unmount());
    globalThis.fetch = realFetch;
    notices.length = 0;
});
afterAll(() => {
    info.mockRestore();
    GlobalRegistrator.unregister();
});

function Reader() {
    const { slug = '' } = useParams<{ slug: string }>();
    const from = new URLSearchParams(useSearch()).get('from');
    useEntrySteps(slug, from);
    return null;
}

async function open(href: string, respond: (path: string, call: number) => unknown) {
    const requested: string[] = [];
    globalThis.fetch = Object.assign(async (input: RequestInfo | URL) => {
        const path = String(input);
        requested.push(path);
        return Response.json(await respond(path, requested.length));
    }, { preconnect: realFetch.preconnect });
    window.history.replaceState(null, '', href);
    const container = document.createElement('div');
    const root = createRoot(container);
    await act(async () => root.render(
        <Router hook={useAppLocation}>
            <Route path="/entry/:slug"><Reader /></Route>
        </Router>,
    ));
    unmount = () => { root.unmount(); unmount = () => {}; };
    return requested;
}

async function press(key: string, shiftKey = true) {
    await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey }));
        await Bun.sleep(0);
    });
}

const here = () => window.location.pathname + window.location.search;

test('Shift+Down and Shift+Up step through the digest day the entry came from', async () => {
    const requested = await open('/entry/b?from=2026-10-14', () => ({ date: '2026-10-14', hasActiveJob: false, entries: [{ entrySlug: 'a' }, { entrySlug: 'b' }, { entrySlug: 'c' }] }));

    await press('ArrowDown');
    expect(here()).toBe('/entry/c?from=2026-10-14');
    await press('ArrowDown');
    expect(here()).toBe('/entry/c?from=2026-10-14');
    expect(notices).toEqual(['No next article in this digest']);
    await press('k', false);
    await press('k', false);
    expect(here()).toBe('/entry/a?from=2026-10-14');
    await press('j', false);
    expect(here()).toBe('/entry/b?from=2026-10-14');
    expect(requested).toHaveLength(2);
    expect(requested[0]).toStartWith('/api/digest?date=2026-10-14&');
});

test('an entry opened from a newsletter steps through that newsletter, and stops at its first issue', async () => {
    const requested = await open('/entry/second?from=%2Fsources%2Fnewsletters%2Fnl_example', () => [{ entrySlug: 'first' }, { entrySlug: 'second' }]);

    await press('ArrowUp');
    expect(here()).toBe('/entry/first?from=%2Fsources%2Fnewsletters%2Fnl_example');
    await press('ArrowUp');
    expect(notices).toEqual(['No previous article from this source']);
    expect(requested).toEqual(['/api/newsletters/nl_example/entries', '/api/newsletters/nl_example/entries']);
});

test('an entry collected after the list was loaded is still reached', async () => {
    const digest = (slugs: string[]) => ({ date: '2026-10-14', hasActiveJob: false, entries: slugs.map((entrySlug) => ({ entrySlug })) });
    const requested = await open('/entry/a?from=2026-10-14', (_, call) => digest(call === 1 ? ['a', 'b'] : ['a', 'b', 'c']));

    await press('j', false);
    await press('j', false);
    expect(here()).toBe('/entry/c?from=2026-10-14');
    expect(requested).toHaveLength(2);
    expect(notices).toEqual([]);
});

test('presses made while the list loads each move one entry', async () => {
    const pushes = spyOn(window.history, 'pushState');
    await open('/entry/a?from=2026-10-14', async () => {
        await Bun.sleep(30);
        return { date: '2026-10-14', hasActiveJob: false, entries: [{ entrySlug: 'a' }, { entrySlug: 'b' }, { entrySlug: 'c' }] };
    });

    await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }));
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j' }));
        await Bun.sleep(80);
    });
    expect(here()).toBe('/entry/c?from=2026-10-14');
    expect(pushes).toHaveBeenCalledTimes(2);
    pushes.mockRestore();
});
