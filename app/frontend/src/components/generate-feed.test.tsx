import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEventTarget = EventTarget;
const nativeEvent = Event;
const zone = process.env.TZ;
// West of UTC, a day-precision date read in local time lands on the previous day.
process.env.TZ = 'America/Los_Angeles';
GlobalRegistrator.register({ url: 'http://app.example.com/' });
const DOMEvent = Event;
// The shared source event bus outlives this file's DOM window, and native targets reject DOM events.
Object.assign(globalThis, { EventTarget: nativeEventTarget, Event: nativeEvent });
// happy-dom rejects a cancelled animation's promise unhandled; without WAAPI, motion animates on its own frame loop.
Reflect.deleteProperty(Element.prototype, 'animate');
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { AddSourceDialog } = await import('./add-source-dialog');
const { AppProfileContext } = await import('@/lib/app-profile-context');

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    globalThis.fetch = realFetch;
});

afterAll(() => {
    GlobalRegistrator.unregister();
    process.env.TZ = zone;
});

const preview = {
    id: 'abc',
    feedUrl: 'https://app.example.com/feed/abc',
    title: 'Example Publication',
    siteUrl: 'https://news.example.com/',
    health: 'ok',
    healthReason: '',
    needsAnchor: false,
    items: [
        { url: 'https://news.example.com/posts/first', title: 'First post', date: '2026-10-09T08:00:00Z' },
        { url: 'https://news.example.com/posts/second', title: 'Second post', date: null },
    ],
    itemCount: 12,
};

type Call = { url: string; body: Record<string, unknown> };

// The lookup finds no feed, so the sheet offers to generate one; handler answers generation and saves.
function respond(handler: (call: Call) => Response | Promise<Response>) {
    const calls: Call[] = [];
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: RequestInit) => {
            const call = { url: String(url), body: JSON.parse(String(init?.body ?? '{}')) };
            if (call.url.endsWith('/resolve')) return Response.json({ candidates: [], existingSubscriptions: [] });
            if (call.url.endsWith('/profile')) return Response.json({ profile: null });
            if (call.url.endsWith('/tags')) return Response.json({ tags: [] });
            calls.push(call);
            return handler(call);
        },
        { preconnect: realFetch.preconnect },
    );
    return calls;
}

// The screen on top of the sheet; screens below it stay mounted but hidden.
function screen() {
    return [...document.querySelectorAll<HTMLElement>('.overscroll-contain > [tabindex="-1"]')].find((node) => !node.hidden)!;
}

async function waitFor(text: string) {
    for (let tries = 0; tries < 100 && !screen()?.textContent?.includes(text); tries++) {
        await act(async () => {
            await new Promise((resolve) => setTimeout(resolve, 20));
        });
    }
    expect(screen().textContent).toContain(text);
}

async function generate() {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () =>
        root.render(
            <AppProfileContext.Provider value={{ kind: 'loading' }}>
                <AddSourceDialog initialUrl="https://news.example.com/" />
            </AppProfileContext.Provider>,
        ),
    );
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    await waitFor('No feed found');
    await click(button('Generate a feed'));
}

function button(name: string) {
    return [...screen().querySelectorAll('button')].find(
        (item) => item.textContent?.trim() === name || item.getAttribute('aria-label') === name,
    );
}

function anchorField() {
    return screen().querySelector<HTMLInputElement>('input[inputmode="url"]');
}

function nameField() {
    return document.querySelector<HTMLInputElement>('input[aria-label="Name"]');
}

async function click(element: HTMLElement | undefined) {
    expect(element).toBeDefined();
    element!.focus();
    await act(async () => element!.click());
}

async function type(input: HTMLInputElement, value: string) {
    input.focus();
    await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setter.call(input, value);
        input.dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });
}

function announced() {
    return [...screen().querySelectorAll('[role="status"]')].map((node) => node.textContent?.trim()).filter(Boolean);
}

const unsure = { ...preview, health: 'needs_attention', needsAnchor: true };

function watchScrolls() {
    const original = HTMLElement.prototype.scrollIntoView;
    const calls: { text: string | null; options: unknown }[] = [];
    HTMLElement.prototype.scrollIntoView = function (this: HTMLElement, options?: unknown) {
        calls.push({ text: this.textContent, options });
    };
    return { calls, restore: () => (HTMLElement.prototype.scrollIntoView = original) };
}

async function settle() {
    await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

test('generating opens its own screen, previews the posts, and adds the feed under the name and tags the reader gives it', async () => {
    let release = () => {};
    const calls = respond(({ url }) =>
        url === '/api/web-sources'
            ? new Promise((resolve) => {
                  release = () => resolve(Response.json(preview));
              })
            : Response.json({ rkey: '3example' }),
    );
    await generate();
    expect(document.querySelector('[aria-label="Back"]')).not.toBeNull();
    expect(screen().textContent).not.toContain('No feed found');
    expect(announced()).toEqual(['Reading the page and a few of its posts. This can take a little while.']);
    expect(button('Add to sources')).toBeUndefined();
    expect(nameField()).toBeNull();

    expect(document.activeElement === document.body || !document.activeElement?.isConnected).toBe(false);

    await act(async () => release());
    await settle();
    expect(document.activeElement?.textContent).toBe('Example Publication');
    expect(announced()).toEqual(['Generated feed, 12 posts found.']);
    expect(screen().textContent).toContain('First post');
    expect(screen().textContent).toContain('Second post');
    expect(screen().querySelectorAll('time')).toHaveLength(1);
    expect(screen().textContent).toContain('12 posts found');
    expect(anchorField()).toBeNull();
    expect(nameField()?.value).toBe('Example Publication');

    await type(nameField()!, 'Example Notes');
    const tag = screen().querySelector<HTMLInputElement>('[role="combobox"]')!;
    await type(tag, 'reading');
    await click(button('Add to sources'));
    await settle();
    expect(calls.map((call) => call.url)).toEqual(['/api/web-sources', '/api/subscriptions']);
    expect(calls[0].body).toEqual({ url: 'https://news.example.com/' });
    expect(calls[1].body).toEqual({
        feedUrl: 'https://app.example.com/feed/abc',
        title: 'Example Notes',
        siteUrl: 'https://news.example.com/',
        tags: ['reading'],
    });
});

test('a generated feed added as it is keeps its own name', async () => {
    const calls = respond(({ url }) => Response.json(url === '/api/web-sources' ? preview : { rkey: '3example' }));
    await generate();
    await waitFor('First post');
    await click(button('Add to sources'));
    await settle();
    expect(calls[1].body).toEqual({
        feedUrl: 'https://app.example.com/feed/abc',
        title: 'Example Publication',
        siteUrl: 'https://news.example.com/',
    });
});

test('an unsure list asks for one post, regenerates around it, and can still be added', async () => {
    let release = () => {};
    const calls = respond(({ body }) =>
        body.anchorUrl
            ? new Promise((resolve) => {
                  release = () =>
                      resolve(Response.json({ ...preview, items: [{ url: 'https://news.example.com/posts/third', title: 'Third post', date: null }], itemCount: 3 }));
              })
            : Response.json(unsure),
    );
    await generate();
    await settle();
    expect(screen().textContent).toContain('Morgenblau isn’t sure these are the site’s posts.');
    expect(button('Add to sources')?.disabled).toBe(false);

    const input = anchorField()!;
    await type(input, 'https://news.example.com/posts/third');
    await act(async () => input.form!.requestSubmit());
    expect(input.disabled).toBe(false);
    expect(input.readOnly).toBe(true);
    expect(input.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(input);
    // happy-dom does not disable a fieldset's controls, so this reads the fieldset browsers honor.
    expect(button('Add to sources')?.closest('fieldset')?.disabled).toBe(true);

    await act(async () => release());
    await settle();
    expect(calls[1].body).toEqual({ url: 'https://news.example.com/', anchorUrl: 'https://news.example.com/posts/third' });
    expect(screen().textContent).toContain('Third post');
    expect(screen().textContent).not.toContain('First post');
    expect(screen().textContent).not.toContain('isn’t sure');
    expect(announced()).toEqual(['Found 3 posts like that one.']);
    expect(document.activeElement).toBe(button('Add to sources')!);
});

for (const [why, line] of [
    ['not_confirmed', 'That post didn’t help. The feed stays as it was.'],
    ['not_on_page', 'That post isn’t linked on this page. The feed stays as it was.'],
    ['no_better', 'That post didn’t help. The feed stays as it was.'],
]) {
    test(`a post that does not help (${why}) leaves the feed and the field as they were`, async () => {
        const calls = respond(({ body }) => Response.json(body.anchorUrl ? { ...unsure, anchorRejected: why } : unsure));
        await generate();
        await settle();
        const input = anchorField()!;
        await type(input, 'https://news.example.com/related/a');
        const scrolled = watchScrolls();
        await act(async () => input.form!.requestSubmit());
        await settle();
        scrolled.restore();

        expect(calls).toHaveLength(2);
        expect(screen().textContent).toContain(line);
        // A tall sheet would otherwise leave the line below its fold.
        expect(scrolled.calls.at(-1)).toEqual({ text: line, options: { block: 'nearest' } });
        expect(announced()).toEqual([line]);
        expect(screen().querySelector('[role="alert"]')).toBeNull();
        expect(input.isConnected).toBe(true);
        expect(input.value).toBe('https://news.example.com/related/a');
        expect(input.getAttribute('aria-invalid')).toBe('false');
        expect(input.getAttribute('aria-describedby')).toBeTruthy();
        expect(document.getElementById(input.getAttribute('aria-describedby')!)?.textContent).toBe(line);
        expect(document.activeElement).toBe(input);
        expect(screen().textContent).toContain('First post');
        expect(screen().textContent).toContain('isn’t sure');
        expect(button('Add to sources')?.disabled).toBe(false);

        await type(input, 'https://news.example.com/posts/third');
        expect(input.form!.textContent).not.toContain(line);
        expect(input.getAttribute('aria-describedby')).toBeNull();
    });
}

test('an unsure list shows fewer posts so the post field stays in view', async () => {
    const items = ['one', 'two', 'three', 'four', 'five'].map((name) => ({ url: `https://news.example.com/posts/${name}`, title: `Post ${name}`, date: null }));
    respond(({ body }) => Response.json(body.anchorUrl ? { ...preview, items } : { ...unsure, items }));
    await generate();
    await settle();
    expect(screen().querySelectorAll('section li')).toHaveLength(3);
});

test('post dates read as the day the site printed, wherever the reader is', async () => {
    respond(() => Response.json({ ...preview, items: [{ ...preview.items[0], date: '2026-10-09T00:00:00Z' }] }));
    await generate();
    await settle();
    expect(screen().querySelector('time')?.textContent).toBe('Oct 9, 2026');
});

test('a page with no posts hands focus to the post address field and offers no name or add', async () => {
    respond(() => Response.json({ ...preview, health: 'broken', needsAnchor: true, items: [], itemCount: 0 }));
    await generate();
    await settle();
    expect(document.activeElement).toBe(anchorField()!);
    expect(nameField()).toBeNull();
    expect(button('Add to sources')).toBeUndefined();
});

test('a site that blocks feed readers offers nothing to add', async () => {
    respond(() => Response.json({ ...preview, health: 'blocked_by_site', needsAnchor: true, items: [], itemCount: 0 }));
    await generate();
    await settle();
    expect(screen().textContent).toContain('This site blocks feed readers');
    expect(button('Add to sources')).toBeUndefined();
    expect(screen().querySelector('input')).toBeNull();
    expect(nameField()).toBeNull();
});

test('a reader over the generation limit is told calmly and not offered a retry', async () => {
    respond(() => Response.json({ code: 'rate_limited', message: 'Slow down' }, { status: 429, headers: { 'Retry-After': '600' } }));
    await generate();
    await settle();
    expect(screen().querySelector('[role="alert"]')?.textContent).toBe('You’ve generated a lot of feeds recently. Try again later.');
    expect(button('Try again')).toBeUndefined();
    expect(screen().contains(document.activeElement)).toBe(true);
});

test('a setup still running from before is waited out with Try again', async () => {
    let attempt = 0;
    respond(() => {
        attempt++;
        return attempt === 1
            ? Response.json({ code: 'setup_in_flight', message: 'Still generating' }, { status: 429, headers: { 'Retry-After': '10' } })
            : Response.json(preview);
    });
    await generate();
    await settle();
    expect(screen().querySelector('[role="alert"]')?.textContent).toBe(
        'Your last feed is still being generated. Try again in a moment.',
    );
    expect(document.activeElement).toBe(button('Try again')!);
    await click(button('Try again'));
    await settle();
    expect(screen().textContent).toContain('First post');
});

test('a shared feed whose last poll failed still offers its stored posts', async () => {
    respond(() => Response.json({ ...preview, health: 'broken', healthReason: 'listing page returned 500' }));
    await generate();
    await settle();
    expect(screen().textContent).toContain('First post');
    expect(screen().textContent).not.toContain('No posts found');
    expect(screen().textContent).toContain('Morgenblau couldn’t read this page on its last visit.');
    expect(button('Add to sources')?.disabled).toBe(false);
    expect(anchorField()).not.toBeNull();
    expect(announced()).toEqual(['Generated feed, 12 posts found.']);
});

test('an unreadable page can be tried again', async () => {
    let attempt = 0;
    respond(() => {
        attempt++;
        return attempt === 1
            ? Response.json({ code: 'upstream_error', message: "Couldn't read that page" }, { status: 502 })
            : Response.json(preview);
    });
    await generate();
    await settle();
    expect(screen().querySelector('[role="alert"]')?.textContent).toBe('Couldn’t read that page.');
    expect(document.activeElement).toBe(button('Try again')!);
    await click(button('Try again'));
    await settle();
    expect(screen().textContent).toContain('First post');
});

test('a server that cannot regenerate keeps the preview and does not blame the address', async () => {
    respond(({ body }) =>
        body.anchorUrl
            ? Response.json({ code: 'unavailable', message: "Feed generation isn't available yet" }, { status: 503 })
            : Response.json(unsure),
    );
    await generate();
    await settle();
    const input = anchorField()!;
    await type(input, 'news.example.com/posts/third');
    await click(button('Use this post'));
    await settle();
    expect(screen().querySelector('[role="alert"]')?.textContent).toBe('Feed generation isn’t available right now.');
    expect(input.getAttribute('aria-invalid')).toBe('false');
    expect(screen().textContent).toContain('First post');
    expect(button('Add to sources')?.disabled).toBe(false);
});

test('a generator that is not available is not offered again at once', async () => {
    respond(() => Response.json({ code: 'unavailable', message: "Feed generation isn't available yet" }, { status: 503 }));
    await generate();
    await settle();
    expect(screen().querySelector('[role="alert"]')?.textContent).toBe('Feed generation isn’t available right now.');
    expect(button('Try again')).toBeUndefined();
});
