import { afterEach, describe, expect, test } from 'bun:test';

import { ApiError } from './api';
import { optimisticDelete } from './optimistic-delete';

const realFetch = globalThis.fetch;

afterEach(() => {
    globalThis.fetch = realFetch;
});

function run(status: number, body?: unknown) {
    globalThis.fetch = (async () =>
        new Response(body === undefined ? null : JSON.stringify(body), {
            status,
        })) as unknown as typeof fetch;

    const calls: string[] = [];
    const errors: unknown[] = [];
    return new Promise<{ calls: string[]; errors: unknown[] }>((resolve) => {
        optimisticDelete({
            path: '/api/saves/abc',
            clear: () => calls.push('clear'),
            restore: () => calls.push('restore'),
            onError: (error) => errors.push(error),
            settle: () => {
                calls.push('settle');
                resolve({ calls, errors });
            },
        });
    });
}

describe('optimisticDelete', () => {
    test('clears immediately and settles without restore on 204', async () => {
        const { calls, errors } = await run(204);
        expect(calls).toEqual(['clear', 'settle']);
        expect(errors).toEqual([]);
    });

    test('restores and reports the ApiError on failure', async () => {
        const { calls, errors } = await run(403, { code: 'reauth_required' });
        expect(calls).toEqual(['clear', 'restore', 'settle']);
        expect(errors).toHaveLength(1);
        expect(errors[0]).toBeInstanceOf(ApiError);
        expect((errors[0] as ApiError).isReauth).toBe(true);
    });

    test('clear runs synchronously before the request resolves', () => {
        globalThis.fetch = (async () =>
            new Response(null, { status: 204 })) as unknown as typeof fetch;
        let cleared = false;
        optimisticDelete({
            path: '/api/saves/abc',
            clear: () => {
                cleared = true;
            },
            restore: () => {},
            settle: () => {},
        });
        expect(cleared).toBe(true);
    });
});
