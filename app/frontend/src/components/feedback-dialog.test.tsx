import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';

GlobalRegistrator.register({ url: 'http://app.example.com/sources' });
// happy-dom rejects a cancelled animation's promise unhandled; without WAAPI, motion animates on its own frame loop.
Reflect.deleteProperty(Element.prototype, 'animate');
const { act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { FeedbackDialog } = await import('./feedback-dialog');

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
const realFetch = globalThis.fetch;
let unmount = () => {};

afterEach(() => {
    act(() => unmount());
    globalThis.fetch = realFetch;
    document.body.innerHTML = '';
});

afterAll(() => GlobalRegistrator.unregister());

async function settle() {
    await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
    });
}

async function openWithDraft(message: string, files: File[]) {
    const container = document.createElement('div');
    document.body.append(container);
    const root = createRoot(container);
    await act(async () => root.render(<FeedbackDialog />));
    unmount = () => root.unmount();

    await act(async () => document.querySelector<HTMLButtonElement>('button[aria-label="Send feedback"]')!.click());
    await settle();
    const textarea = document.querySelector('textarea')!;
    await act(async () => {
        Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, message);
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
    });
    const input = document.querySelector<HTMLInputElement>('input[type="file"]')!;
    await act(async () => {
        Object.defineProperty(input, 'files', { configurable: true, value: files });
        input.dispatchEvent(new Event('change', { bubbles: true }));
    });
    return textarea;
}

function submit() {
    return act(async () => {
        document.querySelector('form')!.requestSubmit();
    });
}

test('sends the message, the page details and only the images that can be attached', async () => {
    let sent: FormData | undefined;
    globalThis.fetch = Object.assign(
        async (_url: Parameters<typeof fetch>[0], init?: RequestInit) => {
            sent = init?.body as FormData;
            return new Response(null, { status: 204 });
        },
        { preconnect: realFetch.preconnect },
    );
    const screenshot = new File([new Uint8Array([137, 80, 78, 71])], 'digest.png', { type: 'image/png' });
    const notes = new File(['plain'], 'notes.pdf', { type: 'application/pdf' });

    await openWithDraft('  The digest skipped Tuesday.  ', [screenshot, notes]);
    expect(document.querySelectorAll('[aria-label="Attached images"] img')).toHaveLength(1);
    expect(document.querySelector('[role="alert"]')?.textContent).toBe(
        'Only PNG, JPEG, GIF and WebP images can be attached.',
    );

    await submit();
    await settle();

    expect(sent?.get('message')).toBe('The digest skipped Tuesday.');
    expect((sent?.getAll('images') as File[]).map((file) => file.name)).toEqual(['digest.png']);
    const context = new Map(JSON.parse(String(sent?.get('context'))) as [string, string][]);
    expect(context.get('Page')).toBe('http://app.example.com/sources');
    expect(document.querySelector('textarea')).toBeNull();
});

test('a failed send keeps the draft and says why', async () => {
    globalThis.fetch = Object.assign(
        async () =>
            Response.json(
                { code: 'upstream_error', message: 'Couldn’t send your feedback. Try again in a moment.' },
                { status: 502 },
            ),
        { preconnect: realFetch.preconnect },
    );
    const screenshot = new File([new Uint8Array([137, 80, 78, 71])], 'digest.png', { type: 'image/png' });

    const textarea = await openWithDraft('The digest skipped Tuesday.', [screenshot]);
    await submit();
    await settle();

    expect(textarea.value).toBe('The digest skipped Tuesday.');
    expect(document.querySelectorAll('[aria-label="Attached images"] img')).toHaveLength(1);
    expect(document.querySelector('[role="alert"]')?.textContent).toBe(
        'Couldn’t send your feedback. Try again in a moment.',
    );
});
