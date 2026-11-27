import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

GlobalRegistrator.register();
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { useDigest } = await import('./use-digest');
const { subscriptionChanges } = await import('@/lib/add-source');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
let cleanup = () => {};
afterEach(() => {
    act(() => cleanup());
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

function Probe({ date }: { date: string }) {
    const state = useDigest(date);
    return <p>{state.status === 'loaded' ? state.data.entries.map((entry) => entry.title).join(',') : state.status}</p>;
}

async function mount(date = '2026-10-08') {
    const container = document.createElement('div');
    const root = createRoot(container);
    await act(async () => root.render(<Probe date={date} />));
    cleanup = () => {
        root.unmount();
        container.remove();
    };
    return { container, root };
}

test('digest keeps collecting until all feed jobs finish, even when it already has entries', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(
        async () => {
            calls++;
            return Response.json({
                date: '2026-10-08',
                hasActiveJob: calls < 3,
                entries: [{ title: calls < 3 ? 'Existing article' : 'Imported video' }],
            });
        },
        { preconnect: realFetch.preconnect },
    );
    const { container } = await mount();
    expect(container.textContent).toBe('Existing article');
    await act(async () => {
        await Bun.sleep(2200);
    });
    expect(calls).toBe(3);
    expect(container.textContent).toBe('Imported video');
    await act(async () => {
        await Bun.sleep(1100);
    });
    expect(calls).toBe(3);
});

test('a subscription change reloads a settled digest and unmount removes the listener', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(
        async () => {
            calls++;
            return Response.json({
                date: '2026-10-08',
                hasActiveJob: false,
                entries: [{ title: calls === 1 ? 'Existing article' : 'Imported video' }],
            });
        },
        { preconnect: realFetch.preconnect },
    );
    const { container } = await mount();
    await act(async () => subscriptionChanges.dispatchEvent(new Event('change')));
    expect(container.textContent).toBe('Imported video');
    act(() => cleanup());
    cleanup = () => {};
    subscriptionChanges.dispatchEvent(new Event('change'));
    expect(calls).toBe(2);
});

test('a transient polling failure preserves entries and retries until collection finishes', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(
        async () => {
            if (++calls === 2) throw new TypeError('Network error');
            return Response.json({
                date: '2026-10-08',
                hasActiveJob: calls === 1,
                entries: [{ title: calls === 1 ? 'Existing article' : 'Imported video' }],
            });
        },
        { preconnect: realFetch.preconnect },
    );
    const { container } = await mount();
    await act(async () => {
        await Bun.sleep(1100);
    });
    expect(container.textContent).toBe('Existing article');
    await act(async () => {
        await Bun.sleep(1100);
    });
    expect(container.textContent).toBe('Imported video');
    expect(calls).toBe(3);
});

test('a same-date subscription refresh preserves active collection across a failed revalidation', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(
        async () => {
            if (++calls === 2) throw new TypeError('Network error');
            return Response.json({
                date: '2026-10-08',
                hasActiveJob: calls === 1,
                entries: [{ title: calls === 1 ? 'Existing article' : 'Imported video' }],
            });
        },
        { preconnect: realFetch.preconnect },
    );
    const { container } = await mount();
    await act(async () => subscriptionChanges.dispatchEvent(new Event('change')));
    expect(container.textContent).toBe('Existing article');
    await act(async () => {
        await Bun.sleep(1100);
    });
    expect(container.textContent).toBe('Imported video');
    expect(calls).toBe(3);
});

test('a failed load for a different date does not inherit active collection', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(
        async () => {
            if (++calls > 1) throw new TypeError('Network error');
            return Response.json({ date: '2026-10-08', hasActiveJob: true, entries: [{ title: 'Existing article' }] });
        },
        { preconnect: realFetch.preconnect },
    );
    const { container, root } = await mount();
    await act(async () => root.render(<Probe date="2026-10-07" />));
    expect(container.textContent).toBe('error');
    await act(async () => {
        await Bun.sleep(1100);
    });
    expect(calls).toBe(2);
});

test('date changes and subscription refreshes discard late responses; unmount stops pending polling', async () => {
    const requests: { signal: AbortSignal; resolve: (response: Response) => void }[] = [];
    globalThis.fetch = Object.assign(
        (_url: Parameters<typeof fetch>[0], init?: RequestInit) =>
            new Promise<Response>((resolve) => {
                requests.push({ signal: init!.signal!, resolve });
            }),
        { preconnect: realFetch.preconnect },
    );
    const { container, root } = await mount();
    await act(async () => root.render(<Probe date="2026-10-07" />));
    expect(requests[0].signal.aborted).toBe(true);
    await act(async () => subscriptionChanges.dispatchEvent(new Event('change')));
    expect(requests[1].signal.aborted).toBe(true);
    await act(async () =>
        requests[2].resolve(
            Response.json({ date: '2026-10-07', hasActiveJob: true, entries: [{ title: 'Current video' }] }),
        ),
    );
    await act(async () => {
        for (const request of requests.slice(0, 2)) {
            request.resolve(
                Response.json({ date: '2026-10-08', hasActiveJob: true, entries: [{ title: 'Stale video' }] }),
            );
        }
    });
    expect(container.textContent).toBe('Current video');
    act(() => cleanup());
    cleanup = () => {};
    await Bun.sleep(1100);
    expect(requests[2].signal.aborted).toBe(true);
    expect(requests).toHaveLength(3);
});

test('a day collecting when Morgenblau goes out of reach keeps its copy and backs off from one-second polling', async () => {
    let offlineCalls = 0;
    globalThis.fetch = Object.assign(
        async () => Response.json({ date: '2026-10-08', hasActiveJob: true, entries: [{ title: 'Earlier article' }] }),
        { preconnect: realFetch.preconnect },
    );
    const { container } = await mount();
    globalThis.fetch = Object.assign(
        async () => {
            offlineCalls++;
            throw new TypeError('Failed to fetch');
        },
        { preconnect: realFetch.preconnect },
    );
    await act(async () => {
        await Bun.sleep(4500);
    });
    // Retries at 1, 2 and 4 seconds; polling every second would have made four.
    expect(offlineCalls).toBe(3);
    expect(container.textContent).toBe('Earlier article');
});
