import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

GlobalRegistrator.register();
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Login } = await import('./login');
const { api } = await import('@/lib/api');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
afterEach(() => {
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

test('opening the sign-in page clears the previous reader’s offline copies', async () => {
    globalThis.fetch = (async () => Response.json({ entries: ['an earlier reader’s day'] })) as unknown as typeof fetch;
    await api('/api/digest', { offline: true });

    const root = createRoot(document.body.appendChild(document.createElement('div')));
    await act(async () => root.render(<Login />));

    globalThis.fetch = (async () => {
        throw new TypeError('Failed to fetch');
    }) as unknown as typeof fetch;
    expect(await api('/api/digest', { offline: true }).catch((e: unknown) => e)).toBeInstanceOf(TypeError);
    act(() => root.unmount());
});
