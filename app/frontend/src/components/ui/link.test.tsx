import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';

GlobalRegistrator.register({ url: 'http://app.example.com/' });
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { Link } = await import('./link');
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

afterAll(() => GlobalRegistrator.unregister());

test('only same-tab app paths go through the router; a new-tab or protocol-relative link keeps the browser default', async () => {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    const routed: Record<string, boolean> = {};
    const record = (event: MouseEvent) => {
        const anchor = (event.target as Element).closest('a')!;
        routed[anchor.textContent!] = event.defaultPrevented;
        event.preventDefault();
    };
    window.addEventListener('click', record);
    try {
        await act(async () =>
            root.render(
                <>
                    <Link href="/sources">same tab</Link>
                    <Link href="/entries/example" rel="noopener noreferrer" target="_blank">
                        new tab
                    </Link>
                    <Link href="//cdn.example.com/feed.xml">protocol-relative</Link>
                </>,
            ),
        );
        for (const anchor of container.querySelectorAll('a')) {
            await act(async () => {
                anchor.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, button: 0 }));
            });
        }
        expect(routed).toEqual({ 'same tab': true, 'new tab': false, 'protocol-relative': false });
        expect(window.location.pathname).toBe('/sources');
    } finally {
        window.removeEventListener('click', record);
        await act(async () => root.unmount());
        container.remove();
    }
});
