import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEventTarget = EventTarget;
const nativeEvent = Event;
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

afterAll(() => GlobalRegistrator.unregister());

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
    await act(async () => element!.click());
}

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
    expect(container.querySelector('[role="status"]')?.textContent).toContain('This can take a little while');
    expect(button(container, 'Add source')).toBeUndefined();

    await act(async () => release());
    await settle();
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
    const calls = respond(({ body }) =>
        Response.json(
            body.anchorUrl
                ? { ...preview, items: [{ url: 'https://news.example.com/posts/third', title: 'Third post', date: null }], itemCount: 3 }
                : { ...preview, health: 'needs_attention', needsAnchor: true },
        ),
    );
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.textContent).toContain('This list looks unsure');
    expect(button(container, 'Add source')?.disabled).toBe(false);

    const input = container.querySelector('input')!;
    await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setter.call(input, 'https://news.example.com/posts/third');
        input.dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });
    await click(button(container, 'Use this post'));
    await settle();
    expect(calls[1].body).toEqual({ url: 'https://news.example.com/', anchorUrl: 'https://news.example.com/posts/third' });
    expect(container.textContent).toContain('Third post');
    expect(container.textContent).not.toContain('First post');
    expect(container.textContent).not.toContain('This list looks unsure');
    expect(button(container, 'Add source')).toBeDefined();
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
});

test('an unreachable site can be tried again', async () => {
    let attempt = 0;
    respond(() => {
        attempt++;
        return attempt === 1
            ? Response.json({ code: 'upstream_error', message: 'Couldn’t read that page' }, { status: 502 })
            : Response.json(preview);
    });
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Couldn’t reach that site. Try again later.');
    await click(button(container, 'Try again'));
    await settle();
    expect(container.textContent).toContain('First post');
});

test('a server that cannot regenerate keeps the preview and does not blame the address', async () => {
    respond(({ body }) =>
        body.anchorUrl
            ? Response.json({ code: 'unavailable', message: 'Feed generation isn’t available yet' }, { status: 503 })
            : Response.json({ ...preview, health: 'needs_attention', needsAnchor: true }),
    );
    const { container } = await render();
    await click(button(container, 'Generate a feed'));
    await settle();
    const input = container.querySelector('input')!;
    await act(async () => {
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setter.call(input, 'news.example.com/posts/third');
        input.dispatchEvent(new DOMEvent('input', { bubbles: true }));
    });
    await click(button(container, 'Use this post'));
    await settle();
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Feed generation isn’t available yet');
    expect(input.getAttribute('aria-invalid')).toBe('false');
    expect(container.textContent).toContain('First post');
    expect(button(container, 'Add source')?.disabled).toBe(false);
});
