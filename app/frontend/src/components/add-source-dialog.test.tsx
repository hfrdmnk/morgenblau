import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEventTarget = EventTarget;
const nativeEvent = Event;
GlobalRegistrator.register({ url: 'http://app.example.com/' });
const DOMEvent = Event;
// The shared source event bus outlives this file's DOM window, and native targets reject DOM events.
Object.assign(globalThis, { EventTarget: nativeEventTarget, Event: nativeEvent });
// happy-dom rejects a cancelled animation's promise unhandled; without WAAPI, motion animates on its own frame loop.
Reflect.deleteProperty(Element.prototype, 'animate');
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { AddSourceDialog, EditSourceSheet } = await import('./add-source-dialog');
const { AppProfileContext } = await import('@/lib/app-profile-context');

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    globalThis.fetch = realFetch;
});

afterAll(() => GlobalRegistrator.unregister());

function respond(handler: (url: string, body: string) => Response) {
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: RequestInit) => handler(String(url), String(init?.body ?? '')),
        { preconnect: realFetch.preconnect },
    );
}

async function settle() {
    await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

// The newsletter panel's exit runs on motion's frame loop (test/preload.ts) before results mount.
async function waitFor(text: string) {
    for (let tries = 0; tries < 100 && !document.body.textContent?.includes(text); tries++) {
        await act(async () => {
            await new Promise((resolve) => setTimeout(resolve, 20));
        });
    }
    expect(document.body.textContent).toContain(text);
}

async function findFeeds(site: string) {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () =>
        root.render(
            <AppProfileContext.Provider value={{ kind: 'loading' }}>
                <AddSourceDialog />
            </AppProfileContext.Provider>,
        ),
    );
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Add a source"]')!.click());
    await settle();
    const input = document.querySelector<HTMLInputElement>('#source-url')!;
    await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setter.call(input, site);
        input.dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });
    await act(async () => input.form!.requestSubmit());
}

function announced() {
    return [...document.querySelectorAll('[role="status"]')].map((node) => node.textContent?.trim()).filter(Boolean);
}

test('the generate offer is not read out wholesale by a surrounding live region', async () => {
    respond(() => Response.json({ candidates: [], existingSubscriptions: [] }));
    await findFeeds('news.example.com');
    await waitFor('No feed found');
    const offer = [...document.querySelectorAll('p')].find((node) => node.textContent?.startsWith('No feed found'));
    expect(offer).toBeDefined();
    expect(offer!.closest('[aria-live]')).toBeNull();
});

test('a page without a feed is announced along with the offer to generate one', async () => {
    respond(() => Response.json({ candidates: [], existingSubscriptions: [] }));
    await findFeeds('news.example.com');
    await waitFor('No feed found');
    expect(announced()).toEqual(['No feed found. You can generate one.']);
});

test('found feeds are announced in one short line', async () => {
    respond(() =>
        Response.json({
            candidates: [
                { feedUrl: 'https://news.example.com/rss.xml', title: 'Example Publication' },
                { feedUrl: 'https://news.example.com/atom.xml', title: 'Example Publication (Atom)' },
            ],
            existingSubscriptions: [],
        }),
    );
    await findFeeds('news.example.com');
    await waitFor('Available feeds');
    expect(document.querySelector('ul[aria-labelledby]')!.closest('[aria-live]')).toBeNull();
    expect(announced()).toEqual(['2 feeds found.']);
});

function screen() {
    return [...document.querySelectorAll<HTMLElement>('.overscroll-contain > [tabindex="-1"]')].find((node) => !node.hidden)!;
}

async function press(text: string) {
    await waitFor(text);
    const button = [...screen().querySelectorAll('button')].find((node) => node.textContent?.includes(text));
    expect(button).toBeDefined();
    await act(async () => button!.click());
    await settle();
}

async function back() {
    await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Back"]')!.click());
    await settle();
}

test('the publication follows its site’s second RSS feed through removal and re-adding in the sheet', async () => {
    const atom = 'https://news.example.com/atom.xml';
    respond((url) => {
        if (url.endsWith('/resolve'))
            return Response.json({
                candidates: [
                    { feedUrl: 'https://news.example.com/rss.xml', title: 'Example RSS' },
                    { feedUrl: atom, title: 'Example Atom' },
                    { kind: 'standardfeed', publication: 'at://did:plc:example/site.standard.publication/self', title: 'Example Standard', subscribedVia: { kind: 'rss' } },
                ],
                existingSubscriptions: [{ feedUrl: atom, rkey: 'atom-rkey', title: 'Example Atom' }],
            });
        if (url.endsWith('/profile')) return Response.json({ profile: null });
        if (url.endsWith('/preview')) return Response.json({ items: [] });
        if (url.endsWith('/tags')) return Response.json({ tags: [] });
        if (url.endsWith('/atom-rkey')) return new Response(null, { status: 204 });
        if (url.endsWith('/api/subscriptions')) return Response.json({ rkey: 'atom-again' });
        throw new Error(`unexpected ${url}`);
    });
    const followedNote = 'You follow this site through its RSS feed';
    await findFeeds('news.example.com');

    await press('Example Standard');
    await waitFor(followedNote);
    expect(screen().textContent).toContain(followedNote);

    await back();
    await press('Example Atom');
    await press('Remove from sources');
    await back();
    await press('Example Standard');
    expect(screen().textContent).not.toContain(followedNote);
    expect(screen().textContent).toContain('Add to sources');

    await back();
    await press('Example Atom');
    await press('Add to sources');
    await press('Example Standard');
    expect(screen().textContent).toContain(followedNote);
});

test('a YouTube channel added without Shorts follows its long-form feed and reads as already followed', async () => {
    const all = 'https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv';
    const longForm = 'https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv';
    const previewed: string[] = [];
    let added = '';
    respond((url, body) => {
        if (url.endsWith('/resolve'))
            return Response.json({
                candidates: [{ feedUrl: all, title: 'Example Channel' }],
                existingSubscriptions: [],
            });
        if (url.endsWith('/profile')) return Response.json({ profile: null });
        if (url.endsWith('/preview')) {
            previewed.push(JSON.parse(body).feedUrl);
            return Response.json({ items: [] });
        }
        if (url.endsWith('/tags')) return Response.json({ tags: [] });
        if (url.endsWith('/api/subscriptions')) {
            added = JSON.parse(body).feedUrl;
            return Response.json({ rkey: 'long-rkey' });
        }
        throw new Error(`unexpected ${url}`);
    });
    await findFeeds('https://www.youtube.com/@example');
    await press('Example Channel');
    await waitFor('Exclude Shorts');
    const label = [...screen().querySelectorAll('label')].find((node) => node.textContent?.includes('Exclude Shorts'))!;
    const toggle = screen().querySelector<HTMLElement>(`[role="switch"][aria-labelledby="${label.id}"]`)!;
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await act(async () => toggle.click());
    await settle();
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(previewed).toEqual([all, longForm]);

    await press('Add to sources');
    expect(added).toBe(longForm);
    await waitFor('In your sources');
});

test('editing a channel followed without Shorts renames it and brings Shorts back in one change', async () => {
    const all = 'https://www.youtube.com/feeds/videos.xml?channel_id=UCabcdefghijklmnopqrstuv';
    const longForm = 'https://www.youtube.com/feeds/videos.xml?playlist_id=UULFabcdefghijklmnopqrstuv';
    const previewed: string[] = [];
    let patched: unknown;
    let changed = 0;
    respond((url, body) => {
        if (url.endsWith('/preview')) {
            previewed.push(JSON.parse(body).feedUrl);
            return Response.json({ items: [] });
        }
        if (url.endsWith('/tags')) return Response.json({ tags: [] });
        if (url.endsWith('/api/subscriptions/long-rkey')) {
            patched = JSON.parse(body);
            return Response.json({ rkey: 'long-rkey' });
        }
        throw new Error(`unexpected ${url}`);
    });
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    const source = { rkey: 'long-rkey', kind: 'rss' as const, feedUrl: longForm, title: 'Example Channel', primary: false, tags: ['video'], fetchStatus: 'ready' as const };
    await act(async () => root.render(<EditSourceSheet onChanged={() => changed++} source={source} />));
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="Edit Example Channel"]')!.click());
    await waitFor('Exclude Shorts');
    const saveButton = () => [...screen().querySelectorAll('button')].find((node) => node.textContent?.includes('Save changes'))!;
    expect(saveButton().disabled).toBe(true);
    const label = [...screen().querySelectorAll('label')].find((node) => node.textContent?.includes('Exclude Shorts'))!;
    const toggle = screen().querySelector<HTMLElement>(`[role="switch"][aria-labelledby="${label.id}"]`)!;
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    await act(async () => toggle.click());
    await settle();
    expect(saveButton().disabled).toBe(false);
    await act(async () => toggle.click());
    await settle();
    expect(saveButton().disabled).toBe(true);
    await act(async () => toggle.click());
    await settle();

    const name = document.querySelector<HTMLInputElement>('input[aria-label="Name"]')!;
    expect(name.value).toBe('Example Channel');
    // The name sits in the header, so only its form owner lets Enter in it save.
    expect(name.form?.contains(saveButton())).toBe(true);
    await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setter.call(name, 'Example Videos');
        name.dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });
    await press('Save changes');
    expect(previewed).toEqual([longForm, all, longForm, all]);
    expect(patched).toEqual({ title: 'Example Videos', feedUrl: all });
    expect(changed).toBe(1);
});

test('an untitled feed with a long address can be added and edited without renaming it', async () => {
    const long = `https://news.example.com/feed.xml?${'utm_source=example&'.repeat(8)}x=1`;
    const tooLong = 'N'.repeat(129);
    const bodies: unknown[] = [];
    respond((url, body) => {
        if (url.endsWith('/resolve')) return Response.json({ candidates: [{ feedUrl: long }], existingSubscriptions: [] });
        if (url.endsWith('/profile')) return Response.json({ profile: null });
        if (url.endsWith('/preview')) return Response.json({ items: [] });
        if (url.endsWith('/tags')) return Response.json({ tags: [] });
        if (url.endsWith('/api/subscriptions/long-rkey') || url.endsWith('/api/subscriptions')) {
            bodies.push(JSON.parse(body));
            return Response.json({ rkey: 'long-rkey' });
        }
        throw new Error(`unexpected ${url}`);
    });
    const button = (text: string) => [...screen().querySelectorAll('button')].find((node) => node.textContent?.includes(text))!;
    const name = () => document.querySelector<HTMLInputElement>('input[aria-label="Name"]')!;
    const type = async (value: string) => act(async () => {
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(name(), value);
        name().dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });

    await findFeeds('news.example.com');
    await press(long);
    await waitFor('Add to sources');
    expect(name().value).toBe(long);
    expect(name().getAttribute('aria-invalid')).toBe('false');
    expect(button('Add to sources').disabled).toBe(false);
    expect(name().getAttribute('aria-describedby')).toBeNull();
    await type(tooLong);
    expect(name().getAttribute('aria-invalid')).toBe('true');
    expect(button('Add to sources').disabled).toBe(true);
    expect(document.getElementById(name().getAttribute('aria-describedby')!)?.textContent).toContain('Names can be up to 128 characters.');
    await type(long);
    await press('Add to sources');
    expect(bodies[0]).toEqual({ feedUrl: long });
    act(() => unmount());

    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    const source = { rkey: 'long-rkey', kind: 'rss' as const, feedUrl: long, primary: false, fetchStatus: 'waiting' as const };
    await act(async () => root.render(<EditSourceSheet onChanged={() => {}} source={source} />));
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label^="Edit "]')!.click());
    await waitFor('Save changes');
    expect(name().getAttribute('aria-invalid')).toBe('false');
    const primary = [...screen().querySelectorAll('label')].find((node) => node.textContent?.includes('Primary'))!;
    await act(async () => screen().querySelector<HTMLElement>(`[role="switch"][aria-labelledby="${primary.id}"]`)!.click());
    await settle();
    expect(button('Save changes').disabled).toBe(false);
    await press('Save changes');
    expect(bodies[1]).toEqual({ primary: true });
});
