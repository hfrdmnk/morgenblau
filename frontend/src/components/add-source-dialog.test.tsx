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
const { AddSourceDialog } = await import('./add-source-dialog');
const { AppProfileContext } = await import('@/lib/app-profile-context');

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    globalThis.fetch = realFetch;
});

afterAll(() => GlobalRegistrator.unregister());

function respond(handler: (url: string) => Response) {
    globalThis.fetch = Object.assign(
        async (url: Parameters<typeof fetch>[0]) => handler(String(url)),
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
    expect(document.querySelector('#available-feeds')!.closest('[aria-live]')).toBeNull();
    expect(announced()).toEqual(['2 feeds found.']);
});
