import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, setSystemTime, test } from 'bun:test';

GlobalRegistrator.register({ url: 'http://app.example.com/' });
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Link, Router, useLocation } = await import('wouter');
const { AppHeader } = await import('./app-header');
const { useAppLocation, useNavigationScrollRestoration } = await import('@/hooks/use-app-location');
const { DataSettings } = await import('@/layouts/data-settings');
const { AppProfileContext } = await import('@/lib/app-profile-context');

// Wouter's module may be cached from a different Happy DOM window.
for (const method of ['pushState', 'replaceState'] as const) {
    const original = window.history[method].bind(window.history);
    window.history[method] = (...args: Parameters<History[typeof method]>) => {
        original(args[0], args[1], args[2]);
        window.dispatchEvent(new Event('popstate'));
    };
}

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    setSystemTime();
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 0 });
});

afterAll(() => GlobalRegistrator.unregister());

function Page() {
    const [location] = useLocation();
    useNavigationScrollRestoration();
    return location.startsWith('/settings') ? (
        <DataSettings><h1>Settings content</h1></DataSettings>
    ) : (
        <Link href="/settings/general">Open settings</Link>
    );
}

async function render(href: string) {
    window.history.replaceState(null, '', href);
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () => root.render(
        <Router hook={useAppLocation}>
            <AppProfileContext.Provider value={{ kind: 'loading' }}>
                <AppHeader />
                <Page />
            </AppProfileContext.Provider>
        </Router>,
    ));
    unmount = () => {
        root.unmount();
        container.remove();
        unmount = () => {};
    };
    return container;
}

function link(container: HTMLElement, name: string): HTMLAnchorElement {
    const found = [...container.querySelectorAll('a')].find((item) =>
        item.getAttribute('aria-label') === name || item.textContent?.trim() === name,
    );
    expect(found).toBeDefined();
    return found!;
}

test('all three modes are direct links, including from settings', async () => {
    const container = await render('/settings/general');
    const nav = container.querySelector('nav[aria-label="Modes"]');
    expect(nav).not.toBeNull();
    expect(nav!.querySelector('[aria-current]')).toBeNull();
    expect(nav!.querySelectorAll('a')).toHaveLength(3);

    await act(async () => link(container, 'Sources').click());
    expect(window.location.pathname).toBe('/sources');
    expect(link(container, 'Sources').getAttribute('aria-current')).toBe('page');
    await act(async () => link(container, 'Library').click());
    expect(window.location.pathname).toBe('/library');
    expect(link(container, 'Library').getAttribute('aria-current')).toBe('page');
    await act(async () => link(container, 'Digest').click());
    expect(window.location.pathname).toBe('/');
});

test('mobile day controls cross a month boundary and cannot go beyond today', async () => {
    setSystemTime(new Date(2026, 2, 1, 12));
    const container = await render('/');
    expect(container.querySelector('button[aria-label="Next day"]')?.hasAttribute('disabled')).toBe(true);
    expect(link(container, 'Previous day').getAttribute('href')).toBe('/?date=2026-02-28');

    await act(async () => link(container, 'Previous day').click());
    expect(window.location.search).toBe('?date=2026-02-28');
    expect(link(container, 'Next day').getAttribute('href')).toBe('/');
    expect(container.querySelector('nav[aria-label="Digest day"]')?.textContent).not.toContain('Today');
    await act(async () => link(container, 'Next day').click());
    expect(window.location.search).toBe('');
    expect(container.querySelector('button[aria-label="Next day"]')?.hasAttribute('disabled')).toBe(true);
});

test('settings tabs retain the original digest date and return scroll position', async () => {
    const container = await render('/?date=2026-09-29');
    Object.defineProperty(window, 'scrollY', { configurable: true, value: 240 });
    await act(async () => link(container, 'Open settings').click());
    await act(async () => link(container, 'Import').click());
    await act(async () => link(container, 'Export').click());
    expect(container.querySelector('nav[aria-label="Settings"] [aria-current]')?.textContent).toBe('Export');
    expect(link(container, 'Back to Digest').getAttribute('href')).toBe('/?date=2026-09-29');
    await act(async () => link(container, 'Back to Digest').click());
    expect(window.location.pathname + window.location.search).toBe('/?date=2026-09-29');
    expect(window.history.state.scrollY).toBe(240);
});

test('same-URL navigation cancels pending settings scroll restoration', async () => {
    const originalObserver = globalThis.ResizeObserver;
    const originalScrollTo = window.scrollTo;
    const resizeCallbacks = new Set<() => void>();
    let maxScroll = 500;
    class ContentResizeObserver extends originalObserver {
        resize: () => void;
        constructor(callback: ResizeObserverCallback) {
            super(callback);
            this.resize = () => callback([], this);
        }
        observe() { resizeCallbacks.add(this.resize); }
        disconnect() { resizeCallbacks.delete(this.resize); }
    }
    globalThis.ResizeObserver = ContentResizeObserver;
    window.scrollTo = (_options?: number | ScrollToOptions, y?: number) => {
        Object.defineProperty(window, 'scrollY', { configurable: true, value: Math.min(y ?? 0, maxScroll) });
    };
    try {
        const container = await render('/');
        window.scrollTo(0, 240);
        await act(async () => link(container, 'Open settings').click());
        maxScroll = 0;
        await act(async () => link(container, 'Back to Digest').click());
        for (const resize of resizeCallbacks) resize();
        expect(window.scrollY).toBe(0);

        maxScroll = 500;
        for (const resize of resizeCallbacks) resize();
        expect(window.scrollY).toBe(240);

        await act(async () => link(container, 'Open settings').click());
        maxScroll = 0;
        await act(async () => link(container, 'Back to Digest').click());
        await act(async () => link(container, 'Digest').click());
        maxScroll = 500;
        for (const resize of resizeCallbacks) resize();
        expect(window.scrollY).toBe(0);
    } finally {
        act(() => unmount());
        globalThis.ResizeObserver = originalObserver;
        window.scrollTo = originalScrollTo;
    }
});

test('settings return names the previous mode and direct entry falls back to digest', async () => {
    const container = await render('/sources');
    await act(async () => link(container, 'Open settings').click());
    expect(link(container, 'Back to Sources').getAttribute('href')).toBe('/sources');
    act(() => unmount());

    const direct = await render('/settings/import');
    expect(link(direct, 'Back to Digest').getAttribute('href')).toBe('/');
});
