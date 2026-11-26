import { afterEach, describe, expect, test } from 'bun:test';

import { api, ApiError, classifyMutationError, connection, describeMutationError, OFFLINE_IMAGES } from './api';

const realFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = realFetch;
});

function stubFetch(status: number, body?: unknown, capture?: { url?: string; init?: RequestInit }) {
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
        if (capture) {
            capture.url = String(url);
            capture.init = init;
        }
        return new Response(body === undefined ? null : JSON.stringify(body), { status });
    }) as typeof fetch;
}

describe('api', () => {
    test('returns the parsed JSON body on 200', async () => {
        stubFetch(200, { handle: 'reader.test' });
        const data = await api<{ handle: string }>('/api/profiles/me');
        expect(data.handle).toBe('reader.test');
    });

    test('returns undefined on 204 no-content', async () => {
        stubFetch(204);
        const data = await api('/api/saves/abc', { method: 'DELETE' });
        expect(data).toBeUndefined();
    });

    test('returns undefined on an empty 200 body', async () => {
        stubFetch(200);
        const data = await api('/api/digest/refresh', { method: 'POST' });
        expect(data).toBeUndefined();
    });

    test('sends JSON body with content-type and same-origin credentials', async () => {
        const capture: { url?: string; init?: RequestInit } = {};
        stubFetch(200, {}, capture);
        await api('/api/subscriptions', {
            method: 'POST',
            body: { feedUrl: 'https://feed.example.test/rss' },
        });
        expect(capture.init?.method).toBe('POST');
        expect(capture.init?.credentials).toBe('same-origin');
        expect(capture.init?.headers).toEqual({
            'content-type': 'application/json',
        });
        expect(capture.init?.body).toBe('{"feedUrl":"https://feed.example.test/rss"}');
    });

    test('sends FormData untouched so the browser sets the multipart boundary', async () => {
        const capture: { url?: string; init?: RequestInit } = {};
        stubFetch(204, undefined, capture);
        const form = new FormData();
        form.append('message', 'Hello');
        await api('/api/feedback', { method: 'POST', body: form });
        expect(capture.init?.body).toBe(form);
        expect(capture.init?.headers).toBeUndefined();
    });

    test('omits content-type header when there is no body', async () => {
        const capture: { url?: string; init?: RequestInit } = {};
        stubFetch(200, {}, capture);
        await api('/api/digest');
        expect(capture.init?.headers).toBeUndefined();
        expect(capture.init?.body).toBeUndefined();
    });

    test('throws ApiError carrying code, message, and field errors', async () => {
        stubFetch(422, {
            code: 'invalid_input',
            message: 'URL is not a feed',
            errors: { url: 'not a feed' },
        });
        const error = await api('/api/subscriptions', {
            method: 'POST',
            body: {},
        }).catch((e: unknown) => e);
        expect(error).toBeInstanceOf(ApiError);
        const apiError = error as ApiError;
        expect(apiError.status).toBe(422);
        expect(apiError.code).toBe('invalid_input');
        expect(apiError.message).toBe('URL is not a feed');
        expect(apiError.errors).toEqual({ url: 'not a feed' });
    });

    test('falls back to a generic message on a body-less failure', async () => {
        stubFetch(502);
        const error = await api('/api/digest').catch((e: unknown) => e);
        expect(error).toBeInstanceOf(ApiError);
        expect((error as ApiError).message).toBe('Request failed (502)');
        expect((error as ApiError).code).toBeUndefined();
    });

    test('isReauth only for 403 + reauth_required', async () => {
        stubFetch(403, { code: 'reauth_required', message: 'Session expired' });
        const reauth = (await api('/api/saves', { method: 'POST', body: {} }).catch((e: unknown) => e)) as ApiError;
        expect(reauth.isReauth).toBe(true);

        stubFetch(403, { code: 'forbidden' });
        const forbidden = (await api('/api/saves').catch((e: unknown) => e)) as ApiError;
        expect(forbidden.isReauth).toBe(false);

        stubFetch(500, { code: 'reauth_required' });
        const serverError = (await api('/api/saves').catch((e: unknown) => e)) as ApiError;
        expect(serverError.isReauth).toBe(false);
    });

    test('lets an abort reject as-is, not as ApiError', async () => {
        globalThis.fetch = (async () => {
            throw new DOMException('aborted', 'AbortError');
        }) as unknown as typeof fetch;
        const error = await api('/api/subscriptions/resolve', {
            method: 'POST',
            body: { url: 'x' },
        }).catch((e: unknown) => e);
        expect(error).toBeInstanceOf(DOMException);
        expect((error as DOMException).name).toBe('AbortError');
    });
});

function stubOffline() {
    globalThis.fetch = (async () => {
        throw new TypeError('Failed to fetch');
    }) as unknown as typeof fetch;
}

describe('api offline copies', () => {
    test('an unreachable network answers each read with its own latest copy', async () => {
        stubFetch(200, { day: 'first' });
        await api('/api/digest?date=2026-11-24', { offline: true });
        stubFetch(200, { day: 'second' });
        await api('/api/digest?date=2026-11-24', { offline: true });
        stubFetch(200, { day: 'other' });
        await api('/api/digest?date=2026-11-23', { offline: true });

        stubOffline();
        expect(await api<unknown>('/api/digest?date=2026-11-24', { offline: true })).toEqual({ day: 'second' });
        expect(await api<unknown>('/api/digest?date=2026-11-23', { offline: true })).toEqual({ day: 'other' });
        const unseen = await api('/api/digest?date=2026-11-22', { offline: true }).catch((e: unknown) => e);
        expect(unseen).toBeInstanceOf(TypeError);
    });

    test('only reads that ask for it keep or use a copy', async () => {
        stubFetch(200, { handle: 'reader.example' });
        await api('/api/profiles/me');
        await api('/api/saves', { offline: true });

        stubOffline();
        expect(await api('/api/profiles/me', { offline: true }).catch((e: unknown) => e)).toBeInstanceOf(TypeError);
        expect(await api('/api/saves').catch((e: unknown) => e)).toBeInstanceOf(TypeError);
    });

    test('a server failure stays a failure instead of showing the copy', async () => {
        stubFetch(200, { entries: [] });
        await api('/api/digest', { offline: true });
        stubFetch(502);
        const error = await api('/api/digest', { offline: true }).catch((e: unknown) => e);
        expect((error as ApiError).status).toBe(502);
    });

    test('an aborted read rejects as aborted even when a copy exists', async () => {
        stubFetch(200, { entries: [] });
        await api('/api/digest', { offline: true });
        const controller = new AbortController();
        controller.abort();
        globalThis.fetch = (async () => {
            throw new DOMException('aborted', 'AbortError');
        }) as unknown as typeof fetch;
        const error = await api('/api/digest', { offline: true, signal: controller.signal }).catch((e: unknown) => e);
        expect((error as DOMException).name).toBe('AbortError');
    });

    test('a 401 from any request clears every copy and every kept image', async () => {
        stubFetch(200, { entries: [] });
        await api('/api/digest', { offline: true });
        await (await caches.open(OFFLINE_IMAGES)).put('/api/entries/abc/image', new Response('image'));
        stubFetch(401, { code: 'unauthenticated' });
        await api('/api/subscriptions/tags').catch(() => {});

        expect(await (await caches.open(OFFLINE_IMAGES)).match('/api/entries/abc/image')).toBeUndefined();
        stubOffline();
        expect(await api('/api/digest', { offline: true }).catch((e: unknown) => e)).toBeInstanceOf(TypeError);
    });

    test('past 300 copies the one written longest ago goes, and rereading a copy keeps it', async () => {
        stubFetch(200, { entries: [] });
        for (let n = 0; n < 300; n++) await api(`/api/entries/e${n}`, { offline: true });
        await api('/api/entries/e0', { offline: true });
        await api('/api/entries/e300', { offline: true });

        stubOffline();
        const read = (n: number) => api(`/api/entries/e${n}`, { offline: true }).catch((e: unknown) => e);
        expect(await read(1)).toBeInstanceOf(TypeError);
        expect(await read(0)).toEqual({ entries: [] });
        expect(await read(2)).toEqual({ entries: [] });
        expect(await read(300)).toEqual({ entries: [] });
    });

    test('the connection turns unreachable on a failed request and back on any answer, but not on an abort', async () => {
        stubFetch(200, { entries: [] });
        await api('/api/digest', { offline: true });
        expect(connection.reachable).toBe(true);
        const changes: boolean[] = [];
        const record = () => changes.push(connection.reachable);
        connection.addEventListener('change', record);

        globalThis.fetch = (async () => {
            throw new DOMException('aborted', 'AbortError');
        }) as unknown as typeof fetch;
        const controller = new AbortController();
        controller.abort();
        await api('/api/digest', { signal: controller.signal }).catch(() => {});
        expect(connection.reachable).toBe(true);

        stubOffline();
        await api('/api/digest', { offline: true });
        expect(connection.reachable).toBe(false);
        stubFetch(503);
        await api('/api/digest').catch(() => {});
        expect(connection.reachable).toBe(true);

        stubOffline();
        await api('/api/digest').catch(() => {});
        dispatchEvent(new Event('online'));
        expect(connection.reachable).toBe(true);
        connection.removeEventListener('change', record);
        expect(changes).toEqual([false, true, false, true]);
    });
});

describe('classifyMutationError', () => {
    test('classifies a reauth ApiError as reauth', () => {
        const error = new ApiError(403, 'reauth_required', 'Session expired');
        expect(classifyMutationError(error)).toBe('reauth');
    });

    test('classifies any other ApiError as failed', () => {
        const error = new ApiError(422, 'invalid_input', 'URL is not a feed');
        expect(classifyMutationError(error)).toBe('failed');
    });

    test('classifies a non-ApiError as failed', () => {
        expect(classifyMutationError(new Error('network down'))).toBe('failed');
    });
});

describe('describeMutationError', () => {
    test('uses the backend message for an ApiError', () => {
        const error = new ApiError(422, 'invalid_input', 'URL is not a feed');
        expect(describeMutationError(error, 'fallback')).toBe('URL is not a feed');
    });

    test('falls back to the caller copy for a non-ApiError', () => {
        expect(describeMutationError(new Error('network down'), 'fallback')).toBe('fallback');
    });
});
