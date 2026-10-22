import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

const nativeEvents = { Event, EventTarget };
GlobalRegistrator.register();
Object.assign(globalThis, nativeEvents);
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { useSourceImport } = await import('./use-source-import');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const realFetch = globalThis.fetch;
let cleanup = () => {};
afterEach(() => {
    act(() => cleanup());
    globalThis.fetch = realFetch;
});
afterAll(() => GlobalRegistrator.unregister());

const sources = [
    { feedUrl: 'https://one.example.com/feed.xml', title: 'Example Publication' },
    { feedUrl: 'https://two.example.com/feed.xml', title: 'Example Video Channel' },
];

function Probe() {
    const { state, startImport, cancel } = useSourceImport();
    return <>
        <button onClick={() => void startImport({ sources, warnings: [] })}>Import</button>
        {state.kind === 'done' && <button onClick={() => void startImport({ sources: state.outcome.remaining, warnings: [] }, state.outcome)}>Retry</button>}
        <button onClick={cancel}>Done</button>
        <output>{JSON.stringify(state)}</output>
    </>;
}

test('the saved receipt stays open and a partial retry retains all confirmed sources for collection feedback', async () => {
    let calls = 0;
    globalThis.fetch = Object.assign(async () => Response.json({
        added: 1, updated: 0, unchanged: 0,
        failures: ++calls === 1 ? [{ feedUrl: sources[1].feedUrl, message: 'Unavailable' }] : [],
    }), { preconnect: realFetch.preconnect });
    const container = document.createElement('div');
    const root = createRoot(container);
    cleanup = () => { root.unmount(); container.remove(); };
    await act(async () => root.render(<Probe />));
    await act(async () => container.querySelector('button')!.click());
    const first = JSON.parse(container.querySelector('output')!.textContent!);
    expect(first.kind).toBe('done');
    expect(first.outcome.imported).toEqual([sources[0]]);
    await act(async () => Array.from(container.querySelectorAll('button')).find((button) => button.textContent === 'Retry')!.click());
    const retried = JSON.parse(container.querySelector('output')!.textContent!);
    expect(retried.kind).toBe('done');
    expect(retried.outcome.added).toBe(2);
    expect(retried.outcome.imported).toEqual(sources);
    expect(retried.outcome.remaining).toEqual([]);
    await act(async () => Array.from(container.querySelectorAll('button')).find((button) => button.textContent === 'Done')!.click());
    expect(JSON.parse(container.querySelector('output')!.textContent!).kind).toBe('idle');
});
