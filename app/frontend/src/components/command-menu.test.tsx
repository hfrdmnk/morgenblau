import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, setSystemTime, test } from 'bun:test';

const nativeEventTarget = EventTarget;
GlobalRegistrator.register({ url: 'http://app.example.com/' });
// The shared source event bus outlives this file's DOM window.
Object.assign(globalThis, { EventTarget: nativeEventTarget });
Reflect.deleteProperty(Element.prototype, 'animate');
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Router } = await import('wouter');
const { CommandMenu } = await import('./command-menu');
const { useAppLocation } = await import('@/hooks/use-app-location');
const { useCommands } = await import('@/hooks/use-commands');
const { CommandProblem } = await import('@/lib/commands');

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
    setSystemTime();
});

afterAll(() => GlobalRegistrator.unregister());

function json(body: unknown) {
    return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } });
}

// The menu's own lists load on open; this reader follows nothing.
function quietServer() {
    globalThis.fetch = Object.assign(async (url: Parameters<typeof fetch>[0]) => {
        const path = String(url);
        if (path === '/api/profiles/me') return json({ did: 'did:plc:example', handle: 'reader.example', displayName: null, avatar: null });
        if (path === '/api/newsletters/address') return json({ address: 'reader@inbound.example.com' });
        if (path.startsWith('/api/newsletters')) return json({ active: [], stopped: [] });
        if (path.endsWith('/tags')) return json({ tags: [] });
        if (path.startsWith('/api/public-following')) return json({ enabled: false });
        return json([]);
    }, { preconnect: realFetch.preconnect });
}

const renames: string[] = [];
const primaryWrites: boolean[] = [];
const tagWrites: string[][] = [];
let failTagWrite = () => {};

function SourcePage() {
    useCommands([{ id: 'headline', group: 'Headlines', label: 'Last Monday in review', searchOnly: true, run: () => {} }, {
        id: 'rename',
        group: 'Example Publication',
        label: 'Rename',
        step: {
            kind: 'text',
            title: 'Rename',
            initial: 'Example Publication',
            placeholder: 'A name',
            action: (value) => `Rename to “${value}”`,
            submit: (value) => {
                renames.push(value);
                return 'Source renamed';
            },
        },
    }, {
        id: 'primary',
        group: 'Example Publication',
        label: 'Primary',
        // Never settles, so every later Enter lands while the first write is still in flight.
        toggle: { on: false, set: (on) => { primaryWrites.push(on); return new Promise(() => {}); } },
    }, {
        id: 'tags',
        group: 'Example Publication',
        label: 'Edit tags',
        step: {
            kind: 'tags',
            title: 'Tags',
            tags: ['design'],
            suggestionsPath: '/api/subscriptions/tags',
            // A write with “weekend” waits until the test fails it, as a save that already toasted its own failure does.
            save: (tags) => {
                tagWrites.push(tags);
                if (!tags.includes('weekend')) return Promise.resolve();
                return new Promise((_, reject) => { failTagWrite = () => reject(new CommandProblem('')); });
            },
        },
    }]);
    return <input aria-label="A field on the page" />;
}

async function render() {
    window.history.replaceState(null, '', '/sources/example');
    quietServer();
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () => root.render(
        <Router hook={useAppLocation}>
            <SourcePage />
            <CommandMenu />
        </Router>,
    ));
    unmount = () => {
        root.unmount();
        container.remove();
    };
}

async function press(key: string, init: KeyboardEventInit = {}) {
    const target = document.activeElement ?? document.body;
    await act(async () => {
        target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }));
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

async function type(text: string) {
    const input = document.activeElement as HTMLInputElement;
    await act(async () => {
        const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
        setValue.call(input, text);
        input.dispatchEvent(new Event('input', { bubbles: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

function menuInput() {
    return document.querySelector<HTMLInputElement>('[cmdk-input]');
}

test('Ctrl+K opens the menu from a text field, and a typed day goes to that digest', async () => {
    setSystemTime(new Date(2026, 10, 7, 9));
    await render();
    document.querySelector<HTMLInputElement>('input[aria-label="A field on the page"]')!.focus();

    await press('k', { ctrlKey: true });
    expect(document.activeElement).toBe(menuInput());

    await type('last monday');
    expect(document.querySelector('[cmdk-item][data-selected="true"]')?.textContent).toContain('Go to Monday, November 2');
    await press('Enter');
    expect(`${location.pathname}${location.search}`).toBe('/?date=2026-11-02');
});

test('closing the menu opened from nowhere leaves focus off the page, not on its first control', async () => {
    await render();
    if (document.activeElement instanceof HTMLElement) document.activeElement.blur();
    await press('k', { metaKey: true });
    await press('Escape');
    expect(document.querySelector('[role="dialog"][data-open]')).toBeNull();
    expect(document.activeElement).toBe(document.body);
});

test('a step edits in place: Escape steps back out, and Enter submits what was typed', async () => {
    await render();
    renames.length = 0;
    await press('k', { metaKey: true });
    expect(document.querySelector('[cmdk-item][data-selected="true"]')?.textContent).toBe('Rename');

    await press('Enter');
    expect(menuInput()?.getAttribute('aria-label')).toBe('Rename');
    expect(menuInput()?.value).toBe('Example Publication');

    await press('Escape');
    expect(menuInput()?.getAttribute('aria-label')).toBe('Search commands');

    await press('Enter');
    await type('Example Weekly');
    await press('Enter');
    expect(renames).toEqual(['Example Weekly']);
    expect(document.querySelector('[role="dialog"][data-open]')).toBeNull();
});

test('Enter pressed again while a command is still saving runs it only once', async () => {
    await render();
    primaryWrites.length = 0;
    await press('k', { metaKey: true });
    await type('primary');
    await act(async () => {
        const input = menuInput()!;
        for (let i = 0; i < 3; i++) input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(primaryWrites).toEqual([true]);
});

test('Add a source opens the add sheet, newsletter address included, outside the app shell', async () => {
    await render();
    await press('k', { metaKey: true });
    await type('add a source');
    await press('Enter');
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });

    const sheet = document.querySelector('[role="dialog"][data-open]');
    expect(sheet?.querySelector<HTMLInputElement>('input[value="reader@inbound.example.com"]')).not.toBeNull();
});

test('a failed tag save unchecks the tag and drops the edit queued behind it', async () => {
    await render();
    tagWrites.length = 0;
    await press('k', { metaKey: true });
    await type('edit tags');
    await press('Enter');
    await type('weekend');
    await press('Enter');
    await type('later');
    await press('Enter');
    await act(async () => {
        failTagWrite();
        await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const tagged = () => [...document.querySelectorAll('[cmdk-item]')].filter((row) => row.textContent?.endsWith('Tagged')).map((row) => row.textContent?.replace('Tagged', ''));
    expect(tagWrites).toEqual([['design', 'weekend']]);
    expect(tagged()).toEqual(['design']);
    expect(document.querySelector('[role="dialog"] [role="alert"]')).toBeNull();

    await type('later');
    await press('Enter');
    expect(tagWrites.at(-1)).toEqual(['design', 'later']);
    expect(tagged()).toEqual(['design', 'later']);
});
