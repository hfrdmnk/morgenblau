export type FieldErrors = Record<string, string>;

// Failure body contract: JSON {code, message} with an optional per-field errors map.
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

    // Reauth is exactly 403 + code 'reauth_required'; any other 403 is a plain failure.
    get isReauth(): boolean {
        return this.status === 403 && this.code === 'reauth_required';
    }
}

// Every optimistic mutation collapses its failure into one of these two buckets.
export type MutationErrorKind = 'reauth' | 'failed';

export function classifyMutationError(error: unknown): MutationErrorKind {
    return error instanceof ApiError && error.isReauth ? 'reauth' : 'failed';
}

// Backend messages are user-facing; anything else (network failure, abort) falls back to the caller's copy.
export function describeMutationError(error: unknown, fallback: string): string {
    return error instanceof ApiError ? error.message : fallback;
}

type ApiOptions = {
    method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
    body?: unknown;
    signal?: AbortSignal;
    // Keeps the latest answer on this device and returns it when the network is unreachable.
    offline?: boolean;
};

// Offline copies belong to the signed-in reader, so every way a session ends clears them before another reader can sign in.
const OFFLINE_COPIES = 'morgenblau-offline';
// The service worker keeps the images those copies show here; vite.config.ts names the same cache.
export const OFFLINE_IMAGES = 'morgenblau-offline-images';
// Cache Storage lists entries oldest-written first, so the cap drops what the reader opened longest ago.
const MAX_OFFLINE_COPIES = 300;

export async function clearOfflineCopies(): Promise<void> {
    await Promise.all([caches.delete(OFFLINE_COPIES), caches.delete(OFFLINE_IMAGES)]);
}

// Whether the latest request reached the server; while it did not, pages show offline copies.
export const connection = Object.assign(new EventTarget(), { reachable: true });

function setReachable(reachable: boolean) {
    if (connection.reachable === reachable) return;
    connection.reachable = reachable;
    connection.dispatchEvent(new Event('change'));
}

// Back on the network, the next request decides; until then the app stops calling itself offline.
addEventListener('online', () => setReachable(true));

async function keepOfflineCopy(path: string, response: Response) {
    const cache = await caches.open(OFFLINE_COPIES);
    await cache.put(path, response);
    const keys = await cache.keys();
    await Promise.all(keys.slice(0, -MAX_OFFLINE_COPIES).map((key) => cache.delete(key)));
}

// Asks the server, and answers from this read's offline copy only when the network is unreachable.
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
        // FormData sets its own multipart content type, boundary included.
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
    // Success bodies may be absent (204 on DELETE); parse only what's there.
    const text = await response.text();
    return (text === '' ? undefined : JSON.parse(text)) as T;
}
