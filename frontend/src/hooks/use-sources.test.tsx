import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEvents = { Event, EventTarget };
GlobalRegistrator.register();
Object.assign(globalThis, nativeEvents);
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { useSources } = await import('./use-sources');
const { subscriptionChanges } = await import('@/lib/add-source');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
let cleanup = () => {};
afterEach(() => {
    act(() => cleanup());
    cleanup = () => {};
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

function Probe() {
    const state = useSources<{ fetchStatus: string }>('/api/subscriptions', 10);
    return <p>{state.status === 'loaded' ? state.data.fetchStatus : state.status}</p>;
}

async function mount() {
    const container = document.createElement('div');
    const root = createRoot(container);
    await act(async () => root.render(<Probe />));
    cleanup = () => { root.unmount(); container.remove(); };
    return container;
}

test('health polling preserves the list through a network failure, observes recovery, and stops on unmount', async () => {
    let calls = 0;
    let recovered = false;
    globalThis.fetch = Object.assign(async () => {
        if (++calls === 2) throw new TypeError('Network error');
        return Response.json({ fetchStatus: recovered ? 'ready' : 'unavailable' });
    }, { preconnect: realFetch.preconnect });
    const container = await mount();
    expect(container.textContent).toBe('unavailable');
    await act(async () => { await Bun.sleep(15); });
    expect(container.textContent).toBe('unavailable');
    recovered = true;
    await act(async () => { await Bun.sleep(40); });
    expect(container.textContent).toBe('ready');
    act(() => cleanup());
    cleanup = () => {};
    const completed = calls;
    subscriptionChanges.dispatchEvent(new Event('change'));
    await Bun.sleep(30);
    expect(calls).toBe(completed);
});

test('subscription changes cancel old health requests and ignore their late responses', async () => {
    const requests: { signal: AbortSignal; resolve: (response: Response) => void }[] = [];
    globalThis.fetch = Object.assign((_url: Parameters<typeof fetch>[0], init?: RequestInit) => new Promise<Response>((resolve) => {
        requests.push({ signal: init!.signal!, resolve });
    }), { preconnect: realFetch.preconnect });
    const container = await mount();
    await act(async () => subscriptionChanges.dispatchEvent(new Event('change')));
    expect(requests[0].signal.aborted).toBe(true);
    await act(async () => requests[1].resolve(Response.json({ fetchStatus: 'ready' })));
    await act(async () => requests[0].resolve(Response.json({ fetchStatus: 'unavailable' })));
    expect(container.textContent).toBe('ready');
    act(() => cleanup());
    cleanup = () => {};
    await Bun.sleep(30);
    expect(requests).toHaveLength(2);
});
