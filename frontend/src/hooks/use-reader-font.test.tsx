import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, mock, test } from 'bun:test';

GlobalRegistrator.register();
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { useReaderFont } = await import('./use-reader-font');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

afterAll(() => GlobalRegistrator.unregister());

function FontChoice() {
    const [font, setFont, error] = useReaderFont();
    return <><button onClick={() => setFont(font === 'sans' ? 'serif' : 'sans')}>{font}</button>{error ? <p role="alert">{error}</p> : null}</>;
}

test('reader font defaults to sans, persists both choices, and rejects unknown stored values', async () => {
    localStorage.clear();
    const container = document.createElement('div');
    document.body.append(container);
    let root = createRoot(container);
    try {
        await act(async () => root.render(<FontChoice />));
        expect(container.textContent).toBe('sans');
        await act(async () => container.querySelector('button')!.click());
        expect(container.textContent).toBe('serif');
        expect(localStorage.getItem('morgenblau-reader-font')).toBe('serif');
        act(() => root.unmount());
        root = createRoot(container);
        await act(async () => root.render(<FontChoice />));
        expect(container.textContent).toBe('serif');
        await act(async () => container.querySelector('button')!.click());
        expect(localStorage.getItem('morgenblau-reader-font')).toBe('sans');
        act(() => root.unmount());
        localStorage.setItem('morgenblau-reader-font', 'unknown');
        root = createRoot(container);
        await act(async () => root.render(<FontChoice />));
        expect(container.textContent).toBe('sans');
    } finally {
        act(() => root.unmount());
        container.remove();
        localStorage.clear();
    }
});

test('denied storage access falls back to sans without breaking render', async () => {
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')!;
    Object.defineProperty(globalThis, 'localStorage', {
        configurable: true,
        get() { throw new DOMException('Storage denied', 'SecurityError'); },
    });
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    try {
        await act(async () => root.render(<FontChoice />));
        expect(container.querySelector('button')?.textContent).toBe('sans');
        expect(container.querySelector('[role=alert]')?.textContent).toContain('could not load');
    } finally {
        act(() => root.unmount());
        container.remove();
        Object.defineProperty(globalThis, 'localStorage', descriptor);
    }
});

test('failed writes preserve the saved selection and report failure, then a retry clears it', async () => {
    const storage = localStorage;
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')!;
    storage.setItem('morgenblau-reader-font', 'serif');
    const write = mock(() => {
        throw new DOMException('Storage full', 'QuotaExceededError');
    });
    Object.defineProperty(globalThis, 'localStorage', {
        configurable: true,
        value: { getItem: storage.getItem.bind(storage), setItem: write },
    });
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    try {
        await act(async () => root.render(<FontChoice />));
        expect(container.querySelector('button')?.textContent).toBe('serif');
        await act(async () => container.querySelector('button')!.click());
        expect(write).toHaveBeenCalledWith('morgenblau-reader-font', 'sans');
        expect(container.querySelector('button')?.textContent).toBe('serif');
        expect(localStorage.getItem('morgenblau-reader-font')).toBe('serif');
        expect(container.querySelector('[role=alert]')?.textContent).toContain('could not save');
        Object.defineProperty(globalThis, 'localStorage', descriptor);
        await act(async () => container.querySelector('button')!.click());
        expect(container.querySelector('button')?.textContent).toBe('sans');
        expect(localStorage.getItem('morgenblau-reader-font')).toBe('sans');
        expect(container.querySelector('[role=alert]')).toBeNull();
    } finally {
        act(() => root.unmount());
        container.remove();
        Object.defineProperty(globalThis, 'localStorage', descriptor);
        localStorage.clear();
    }
});
