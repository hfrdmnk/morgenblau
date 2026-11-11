import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';

GlobalRegistrator.register({ url: 'http://app.example.com/' });
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { PostList } = await import('./item-list');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

afterAll(() => GlobalRegistrator.unregister());

test('arrowsOnly leaves j and k alone, and the keys start from the current row', async () => {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    const played: string[] = [];
    const row = (label: string, current = false) => (
        <li aria-current={current ? 'true' : undefined} data-post="">
            <button data-post-link onClick={() => played.push(label)} type="button">{label}</button>
        </li>
    );
    const press = (key: string) => act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })); });
    const selected = () => container.querySelector('[data-selected]')?.textContent;
    try {
        await act(async () => root.render(<PostList arrowsOnly><ol>{row('0:00')}{row('0:30', true)}{row('1:00')}</ol></PostList>));
        await press('j');
        expect(selected()).toBeUndefined();
        await press('ArrowDown');
        expect(selected()).toBe('1:00');
        await press('ArrowUp');
        await press('ArrowUp');
        expect(selected()).toBe('0:00');
        await act(async () => { (document.activeElement as HTMLElement).dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })); });
        expect(played).toEqual(['0:00']);
    } finally {
        act(() => root.unmount());
        container.remove();
    }
});
