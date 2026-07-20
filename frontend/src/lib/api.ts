export type FieldErrors = Record<string, string>;

// Failure body contract: JSON {code, message} with an optional per-field errors map.
type FailureBody = { code?: string; message?: string; errors?: FieldErrors };

export class ApiError extends Error {
    readonly status: number;
    readonly code: string | undefined;
    readonly errors?: FieldErrors;

    constructor(
        status: number,
        code: string | undefined,
        message: string,
        errors?: FieldErrors,
    ) {
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

// Every optimistic mutation (save, share, subscription edit/delete) collapses its failure into one of these two buckets.
export type MutationErrorKind = 'reauth' | 'failed';

export function classifyMutationError(error: unknown): MutationErrorKind {
    return error instanceof ApiError && error.isReauth ? 'reauth' : 'failed';
}

// Backend messages are user-facing; anything else (network failure, abort) falls back to the caller's copy.
export function describeMutationError(error: unknown, fallback: string): string {
    return error instanceof ApiError ? error.message : fallback;
}

type ApiOptions = {
    method?: 'GET' | 'POST' | 'PATCH' | 'DELETE';
    body?: unknown;
    signal?: AbortSignal;
};

export async function api<T = undefined>(
    path: string,
    options: ApiOptions = {},
): Promise<T> {
    const { method = 'GET', body, signal } = options;
    const response = await fetch(path, {
        method,
        credentials: 'same-origin',
        signal,
        ...(body !== undefined && {
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify(body),
        }),
    });
    if (!response.ok) {
        const failure = (await response
            .json()
            .catch(() => null)) as FailureBody | null;
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
