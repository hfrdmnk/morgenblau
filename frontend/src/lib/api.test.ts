import { afterEach, describe, expect, test } from 'bun:test';

import {
    api,
    ApiError,
    classifyMutationError,
    describeMutationError,
} from './api';

const realFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = realFetch;
});

function stubFetch(
    status: number,
    body?: unknown,
    capture?: { url?: string; init?: RequestInit },
) {
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
        if (capture) {
            capture.url = String(url);
            capture.init = init;
        }
        return new Response(
            body === undefined ? null : JSON.stringify(body),
            { status },
        );
    }) as typeof fetch;
}

describe('api', () => {
    test('returns the parsed JSON body on 200', async () => {
        stubFetch(200, { handle: 'alice.bsky.social' });
        const data = await api<{ handle: string }>('/api/profiles/x');
        expect(data.handle).toBe('alice.bsky.social');
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
        await api('/api/follows', {
            method: 'POST',
            body: { handle: 'alice.test' },
        });
        expect(capture.init?.method).toBe('POST');
        expect(capture.init?.credentials).toBe('same-origin');
        expect(capture.init?.headers).toEqual({
            'content-type': 'application/json',
        });
        expect(capture.init?.body).toBe('{"handle":"alice.test"}');
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
        const reauth = (await api('/api/shares', { method: 'POST', body: {} }).catch(
            (e: unknown) => e,
        )) as ApiError;
        expect(reauth.isReauth).toBe(true);

        stubFetch(403, { code: 'forbidden' });
        const forbidden = (await api('/api/shares').catch(
            (e: unknown) => e,
        )) as ApiError;
        expect(forbidden.isReauth).toBe(false);

        stubFetch(500, { code: 'reauth_required' });
        const serverError = (await api('/api/shares').catch(
            (e: unknown) => e,
        )) as ApiError;
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
        expect(classifyMutationError(new Error('network down'))).toBe(
            'failed',
        );
    });
});

describe('describeMutationError', () => {
    test('uses the backend message for an ApiError', () => {
        const error = new ApiError(422, 'invalid_input', 'URL is not a feed');
        expect(describeMutationError(error, 'fallback')).toBe(
            'URL is not a feed',
        );
    });

    test('falls back to the caller copy for a non-ApiError', () => {
        expect(
            describeMutationError(new Error('network down'), 'fallback'),
        ).toBe('fallback');
    });
});
