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
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { GenerateFeed } = await import('./generate-feed');

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
    feedUrl: 'https://app.example.com/web/abc',
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

function respond(handler: (call: Call) => Response | Promise<Response>) {
    const calls: Call[] = [];
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0], init?: RequestInit) => {
            const call = { url: String(url), body: JSON.parse(String(init?.body ?? '{}')) };
            calls.push(call);
            return handler(call);
        },
        { preconnect: realFetch.preconnect },
    );
    return calls;
}

async function render() {
    const saved: string[] = [];
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    function Harness() {
        return (
            <GenerateFeed
                url="https://news.example.com/"
                instant
                saving={false}
                onSavingChange={() => {}}
                onSaved={() => saved.push('saved')}
            />
        );
    }
    await act(async () => root.render(<Harness />));
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    return { container, saved };
}

function button(container: HTMLElement, name: string) {
    return [...container.querySelectorAll('button')].find(
        (item) => item.textContent?.trim() === name || item.getAttribute('aria-label') === name,
    );
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

function announced(container: HTMLElement) {
    return [...container.querySelectorAll('[role="status"]')].map((node) => node.textContent?.trim()).filter(Boolean);
}

const unsure = { ...preview, health: 'needs_attention', needsAnchor: true };

async function settle() {
    await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

test('offers to generate, waits calmly, previews the posts and subscribes through the ordinary path', async () => {
    let release = () => {};
    const calls = respond(({ url }) =>
        url === '/api/web-sources'
            ? new Promise((resolve) => {
                  release = () => resolve(Response.json(preview));
              })
            : Response.json({ rkey: '3example' }),
    );
    const { container, saved } = await render();
    expect(container.textContent).toContain('No feed found');

    await click(button(container, 'Generate a feed'));
    expect(announced(container)).toEqual(['Reading the page and a few of its posts. This can take a little while.']);
    expect(button(container, 'Add source')).toBeUndefined();

    expect(document.activeElement === document.body || !document.activeElement?.isConnected).toBe(false);

    await act(async () => release());
    await settle();
    expect(document.activeElement?.textContent).toBe('Example Publication');
    expect(announced(container)).toEqual(['Generated feed, 12 posts found.']);
    expect(container.textContent).toContain('Example Publication');
    expect(container.textContent).toContain('First post');
    expect(container.textContent).toContain('Second post');
    expect(container.querySelectorAll('time')).toHaveLength(1);
    expect(container.textContent).toContain('12 posts found');
    expect(container.querySelector('input')).toBeNull();

    await click(button(container, 'Add source'));
    await settle();
    expect(calls.map((call) => call.url)).toEqual(['/api/web-sources', '/api/subscriptions']);
    expect(calls[0].body).toEqual({ url: 'https://news.example.com/' });
    expect(calls[1].body).toEqual({
        feedUrl: 'https://app.example.com/web/abc',
        title: 'Example Publication',
        siteUrl: 'https://news.example.com/',
    });
    expect(saved).toEqual(['saved']);
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
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.textContent).toContain('Morgenblau isn’t sure these are the site’s posts.');
    expect(button(container, 'Add source')?.disabled).toBe(false);

    const input = container.querySelector('input')!;
    await type(input, 'https://news.example.com/posts/third');
    await act(async () => input.form!.requestSubmit());
    expect(input.disabled).toBe(false);
    expect(input.readOnly).toBe(true);
    expect(input.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(input);

    await act(async () => release());
    await settle();
    expect(calls[1].body).toEqual({ url: 'https://news.example.com/', anchorUrl: 'https://news.example.com/posts/third' });
    expect(container.textContent).toContain('Third post');
    expect(container.textContent).not.toContain('First post');
    expect(container.textContent).not.toContain('isn’t sure');
    expect(announced(container)).toEqual(['Found 3 posts like that one.']);
    expect(document.activeElement).toBe(button(container, 'Add source')!);
});

test('an unsure list keeps Add source near the posts by showing fewer of them', async () => {
    const items = ['one', 'two', 'three', 'four', 'five'].map((name) => ({ url: `https://news.example.com/posts/${name}`, title: `Post ${name}`, date: null }));
    respond(({ body }) => Response.json(body.anchorUrl ? { ...preview, items } : { ...unsure, items }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelectorAll('li')).toHaveLength(3);
    const add = button(container, 'Add source')!;
    const field = container.querySelector('input')!;
    expect(add.compareDocumentPosition(field) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test('a new result clears a failed save so the new feed can be added', async () => {
    let saves = 0;
    respond(({ url, body }) => {
        if (url === '/api/subscriptions') {
            saves++;
            return Response.json({ code: 'internal_error', message: 'The save couldn’t be confirmed.' }, { status: 500 });
        }
        return Response.json(body.anchorUrl ? { ...preview, itemCount: 4 } : unsure);
    });
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    await click(button(container, 'Add source'));
    await settle();
    expect(saves).toBe(1);
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('The save couldn’t be confirmed.');
    expect(button(container, 'Add source')?.disabled).toBe(true);

    await type(container.querySelector('input')!, 'news.example.com/posts/first');
    await click(button(container, 'Use this post'));
    await settle();
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(button(container, 'Add source')?.disabled).toBe(false);
});

test('post dates read as the day the site printed, wherever the reader is', async () => {
    respond(() => Response.json({ ...preview, items: [{ ...preview.items[0], date: '2026-10-09T00:00:00Z' }] }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelector('time')?.textContent).toBe('Oct 9, 2026');
});

test('a page with no posts hands focus to the post address field', async () => {
    respond(() => Response.json({ ...preview, health: 'broken', needsAnchor: true, items: [], itemCount: 0 }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(document.activeElement).toBe(container.querySelector('input')!);
});

test('a site that blocks feed readers offers nothing to add', async () => {
    respond(() => Response.json({ ...preview, health: 'blocked_by_site', needsAnchor: true, items: [], itemCount: 0 }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.textContent).toContain('This site blocks feed readers');
    expect(button(container, 'Add source')).toBeUndefined();
    expect(container.querySelector('input')).toBeNull();
});

test('a reader over the hourly limit is told calmly and not offered a retry', async () => {
    respond(() => Response.json({ code: 'rate_limited', message: 'Slow down' }, { status: 429, headers: { 'Retry-After': '600' } }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Too many generated feeds this hour. Try again later.');
    expect(button(container, 'Try again')).toBeUndefined();
    expect(container.contains(document.activeElement)).toBe(true);
});

test('an unreadable page can be tried again', async () => {
    let attempt = 0;
    respond(() => {
        attempt++;
        return attempt === 1
            ? Response.json({ code: 'upstream_error', message: "Couldn't read that page" }, { status: 502 })
            : Response.json(preview);
    });
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Couldn’t read that page.');
    expect(document.activeElement).toBe(button(container, 'Try again')!);
    await click(button(container, 'Try again'));
    await settle();
    expect(container.textContent).toContain('First post');
});

test('a server that cannot regenerate keeps the preview and does not blame the address', async () => {
    respond(({ body }) =>
        body.anchorUrl
            ? Response.json({ code: 'unavailable', message: "Feed generation isn't available yet" }, { status: 503 })
            : Response.json(unsure),
    );
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    const input = container.querySelector('input')!;
    await type(input, 'news.example.com/posts/third');
    await click(button(container, 'Use this post'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Feed generation isn’t available right now.');
    expect(input.getAttribute('aria-invalid')).toBe('false');
    expect(container.textContent).toContain('First post');
    expect(button(container, 'Add source')?.disabled).toBe(false);
});

test('a generator that is not available is not offered again at once', async () => {
    respond(() => Response.json({ code: 'unavailable', message: "Feed generation isn't available yet" }, { status: 503 }));
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Feed generation isn’t available right now.');
    expect(button(container, 'Try again')).toBeUndefined();
});
