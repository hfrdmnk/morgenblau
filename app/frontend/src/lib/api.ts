export type FieldErrors = Record<string, string>;

type FailureBody = { code?: string; message?: string; errors?: FieldErrors };

export class ApiError extends Error {
    readonly status: number;
    readonly code: string | undefined;
    readonly errors?: FieldErrors;

    constructor(status: number, code: string | undefined, message: string, errors?: FieldErrors) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
        this.code = code;
        this.errors = errors;
    }

    get isReauth(): boolean {
        return this.status === 403 && this.code === 'reauth_required';
    }
}

export type MutationErrorKind = 'reauth' | 'failed';

export function classifyMutationError(error: unknown): MutationErrorKind {
    return error instanceof ApiError && error.isReauth ? 'reauth' : 'failed';
}

export function describeMutationError(error: unknown, fallback: string): string {
    return error instanceof ApiError ? error.message : fallback;
}

type ApiOptions = {
    method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
    body?: unknown;
    signal?: AbortSignal;
    offline?: boolean;
};

const OFFLINE_COPIES = 'morgenblau-offline';
export const OFFLINE_IMAGES = 'morgenblau-offline-images';
// Cache Storage keeps a cache's entries in the order they were written:
// https://w3c.github.io/ServiceWorker/#batch-cache-operations-algorithm
const MAX_OFFLINE_COPIES = 300;

export async function clearOfflineCopies(): Promise<void> {
    await Promise.all([caches.delete(OFFLINE_COPIES), caches.delete(OFFLINE_IMAGES)]);
}

export const connection = Object.assign(new EventTarget(), { reachable: true });

function setReachable(reachable: boolean) {
    if (connection.reachable === reachable) return;
    connection.reachable = reachable;
    connection.dispatchEvent(new Event('change'));
}

addEventListener('online', () => setReachable(true));

async function keepOfflineCopy(path: string, response: Response) {
    const cache = await caches.open(OFFLINE_COPIES);
    await cache.put(path, response);
    const keys = await cache.keys();
    await Promise.all(keys.slice(0, -MAX_OFFLINE_COPIES).map((key) => cache.delete(key)));
}

async function reach(path: string, init: RequestInit, offline: boolean) {
    try {
        const response = await fetch(path, init);
        setReachable(true);
        return { response, fromCopy: false };
    } catch (error) {
        if (init.signal?.aborted) throw error;
        setReachable(false);
        const copy = offline ? await (await caches.open(OFFLINE_COPIES)).match(path) : undefined;
        if (!copy) throw error;
        return { response: copy, fromCopy: true };
    }
}

export async function api<T = undefined>(path: string, options: ApiOptions = {}): Promise<T> {
    const { method = 'GET', body, signal, offline = false } = options;
    const init: RequestInit = {
        method,
        credentials: 'same-origin',
        signal,
        ...(body instanceof FormData
            ? { body }
            : body !== undefined && {
                  headers: { 'content-type': 'application/json' },
                  body: JSON.stringify(body),
              }),
    };
    const { response, fromCopy } = await reach(path, init, offline);
    if (response.status === 401) await clearOfflineCopies();
    if (offline && response.ok && !fromCopy) await keepOfflineCopy(path, response.clone());
    if (!response.ok) {
        const failure = (await response.json().catch(() => null)) as FailureBody | null;
        throw new ApiError(
            response.status,
            failure?.code,
            failure?.message ?? `Request failed (${response.status})`,
            failure?.errors,
        );
    }
    const text = await response.text();
    return (text === '' ? undefined : JSON.parse(text)) as T;
}
